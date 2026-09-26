package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

const (
	stopTimeout      = 30 * time.Second // SIGTERM → SIGKILL
	healthTimeout    = 60 * time.Second // subida até /healthz com a versão certa
	versionTimeout   = 15 * time.Second // `<bin> --version`
	maxCrashes       = 5                // saídas seguidas antes de desistir
	crashStableAfter = time.Minute      // uptime que zera a contagem de saídas
	maxCrashDelay    = 30 * time.Second
	rollbackWindow   = 24 * time.Hour // troca recente o bastante para rollback por crash loop
)

// launcher é o estado do lançador. Todos os campos são usados só pela
// goroutine de run; os sinais chegam por canais.
type launcher struct {
	cfg      config
	log      *log.Logger
	store    store
	fetch    fetcher
	health   *http.Client
	childEnv []string
	hup      chan struct{} // SIGHUP (ou teste): checar atualização agora

	stopTimeout, healthTimeout time.Duration
	crashes                    crashPolicy
	crashesSinceUpdate         int // saídas do filho desde a última troca, qualquer uptime

	track   string // trilha efetiva (FFCOM_CHANNEL_VERSION ou padrão da semente)
	current string // versão ativa
	child   *child
}

type child struct {
	cmd     *exec.Cmd
	version string
	started time.Time
	done    chan struct{} // fechado quando o processo termina
	err     error         // resultado de Wait, válido depois de done
}

func newLauncher(cfg config, logger *log.Logger) *launcher {
	return &launcher{
		cfg:           cfg,
		log:           logger,
		store:         store{dir: cfg.RuntimeDir},
		fetch:         fetcher{client: &http.Client{}, indexURL: cfg.IndexURL, pub: cfg.PubKey},
		health:        &http.Client{Timeout: 2 * time.Second},
		childEnv:      os.Environ(),
		hup:           make(chan struct{}, 1),
		stopTimeout:   stopTimeout,
		healthTimeout: healthTimeout,
		crashes:       crashPolicy{max: maxCrashes, stableAfter: crashStableAfter, base: time.Second, maxDelay: maxCrashDelay},
	}
}

// run é o laço principal. Devolve o código de saída do processo.
func (l *launcher) run(ctx context.Context) int {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopSignals := notifySignals(l.log, cancel, l.hup)
	defer stopSignals()

	l.log.Printf("runtime %s (%s), índice %s", l.cfg.RuntimeVersion, l.cfg.Platform, l.cfg.IndexURL)
	for _, w := range l.cfg.Warnings {
		l.log.Print(w)
	}

	v, err := l.boot(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return 0
		}
		l.log.Printf("não há versão do server-channel para subir: %v", err)
		return 1
	}
	l.current = v
	l.start(v)

	next := l.nextInterval()
	if err := l.waitHealthy(ctx, v); err == nil {
		l.log.Printf("server-channel %s no ar", v)
		if err := l.store.writeCurrent(v); err != nil {
			l.log.Printf("gravar current: %v", err)
		}
		next = 0 // primeira checagem logo depois do boot saudável
	} else if ctx.Err() == nil {
		l.log.Printf("server-channel %s não respondeu /healthz na partida: %v", v, err)
	}

	timer := time.NewTimer(next)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			l.stopChild()
			l.log.Print("encerrado")
			return 0
		case <-l.hup:
			l.log.Print("SIGHUP: checando atualização agora")
			l.checkAndUpdate(ctx)
		case <-timer.C:
			l.checkAndUpdate(ctx)
			timer.Reset(l.nextInterval())
		case <-l.child.done:
			if ctx.Err() != nil {
				continue // encerrando: o caso ctx.Done cuida
			}
			if code, exit := l.onChildExit(ctx); exit {
				return code
			}
		}
	}
}

func (l *launcher) nextInterval() time.Duration {
	return l.cfg.Interval + time.Duration(rand.Float64()*0.25*float64(l.cfg.Interval))
}

// boot escolhe a versão da partida entre current, a semente e, sem nenhuma
// das duas, um download pelo índice.
func (l *launcher) boot(ctx context.Context) (string, error) {
	if _, err := l.store.prune(l.keepOnBoot()...); err != nil {
		l.log.Printf("limpar versions/: %v", err)
	}
	bad, err := l.store.readBad()
	if err != nil {
		l.log.Printf("ler %s: %v", l.store.badPath(), err)
	}
	cur, err := l.store.readCurrent()
	if err != nil {
		l.log.Printf("ler current: %v", err)
	}
	curOK := false
	if cur != "" {
		if err := l.store.verifyInstalled(cur); err != nil {
			l.log.Printf("versão ativa %s inutilizável: %v", cur, err)
		} else {
			curOK = true
		}
	}
	seed, err := readSeed(l.cfg.SeedDir)
	if err != nil {
		l.log.Printf("semente em %s inutilizável: %v", l.cfg.SeedDir, err)
		seed = ""
	}

	l.track = l.cfg.Track
	if l.track == "" {
		l.track = defaultTrack(seed, cur)
		l.log.Printf("trilha de atualização: %s (padrão; defina FFCOM_CHANNEL_VERSION para mudar)", l.track)
	} else {
		l.log.Printf("trilha de atualização: %s (FFCOM_CHANNEL_VERSION)", l.track)
	}

	v, fromSeed := chooseBoot(cur, curOK, seed, l.track, bad)
	switch {
	case v == "":
		l.log.Print("sem versão instalada nem semente; baixando pelo índice")
		return l.initialInstall(ctx)
	case fromSeed:
		// Versão vinda da semente não tem troca a desfazer.
		if err := l.store.clearLastUpdate(); err != nil {
			l.log.Printf("apagar last-update: %v", err)
		}
		if err := l.installSeedVersion(seed); err != nil {
			if curOK {
				l.log.Printf("instalar semente %s: %v; ficando em %s", seed, err, cur)
				return cur, nil
			}
			return "", fmt.Errorf("instalar semente %s: %w", seed, err)
		}
		if curOK {
			l.log.Printf("imagem trouxe semente %s, mais nova que a ativa %s: trocando de %s para %s", seed, cur, cur, seed)
		} else {
			l.log.Printf("usando semente %s da imagem", seed)
		}
		if removed, err := l.store.prune(seed, cur); err != nil {
			l.log.Printf("podar versions/: %v", err)
		} else if len(removed) > 0 {
			l.log.Printf("versões antigas apagadas: %s", strings.Join(removed, ", "))
		}
		return seed, nil
	default:
		return v, nil
	}
}

// keepOnBoot preserva todas as versões instaladas na limpeza inicial (só
// temporários órfãos saem); a poda de versões acontece depois de uma troca.
func (l *launcher) keepOnBoot() []string {
	entries, _ := os.ReadDir(l.store.versionsDir())
	var keep []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".tmp-") {
			keep = append(keep, e.Name())
		}
	}
	return keep
}

// installSeedVersion copia a semente para versions/, se ainda não estiver
// lá. A semente foi verificada contra o índice no build da imagem.
func (l *launcher) installSeedVersion(seed string) error {
	if l.store.verifyInstalled(seed) == nil {
		return nil
	}
	path := filepath.Join(l.cfg.SeedDir, binName)
	sum, err := sha256File(path)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return l.store.install(seed, f, sum)
}

// initialInstall baixa a versão da trilha quando não há nada para subir.
// Tenta algumas vezes (a troca de assets de channel-stable deixa janelas de
// 404) e desiste para o Docker reiniciar o container.
func (l *launcher) initialInstall(ctx context.Context) (string, error) {
	delay := 5 * time.Second
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if attempt > 1 {
			l.log.Printf("tentativa %d falhou: %v; de novo em %s", attempt-1, lastErr, delay)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
		idx, err := l.fetch.fetchIndex(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		chosen, skipped, err := release.Resolve(idx, l.track, l.cfg.RuntimeVersion, l.cfg.Platform)
		l.logSkipped(skipped)
		if err != nil {
			lastErr = err
			continue
		}
		if err := l.install(ctx, chosen); err != nil {
			lastErr = err
			continue
		}
		return chosen.Version, nil
	}
	return "", lastErr
}

// checkAndUpdate é a checagem periódica: qualquer falha só é logada e a
// próxima tentativa fica para o próximo intervalo.
func (l *launcher) checkAndUpdate(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	target, err := l.findUpdate(ctx)
	if err != nil {
		if ctx.Err() == nil {
			l.log.Printf("checagem de atualização falhou (tenta de novo no próximo intervalo): %v", err)
		}
		return
	}
	if target != nil {
		l.swap(ctx, *target)
	}
}

// findUpdate baixa e verifica o índice e devolve a versão para a qual
// trocar agora, ou nil.
func (l *launcher) findUpdate(ctx context.Context) (*release.Version, error) {
	idx, err := l.fetch.fetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	chosen, skipped, err := release.Resolve(idx, l.track, l.cfg.RuntimeVersion, l.cfg.Platform)
	l.logSkipped(skipped)
	if errors.Is(err, release.ErrNoVersion) {
		l.log.Printf("nenhuma versão no índice para a trilha %s em %s", l.track, l.cfg.Platform)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !shouldSwitch(l.current, chosen.Version, l.track) {
		return nil, nil
	}
	bad, err := l.store.readBad()
	if err != nil {
		return nil, err
	}
	if bad[chosen.Version] {
		l.log.Printf("versão %s já falhou numa troca (arquivo bad); esperando uma mais nova", chosen.Version)
		return nil, nil
	}
	if !l.cfg.AutoUpdate {
		l.log.Printf("versão nova %s disponível (ativa: %s); FFCOM_AUTO_UPDATE=false, não atualiza", chosen.Version, l.current)
		return nil, nil
	}
	return &chosen, nil
}

func (l *launcher) logSkipped(skipped *release.Version) {
	if skipped != nil {
		l.log.Printf("versão %s existe mas exige imagem com runtime >= %s (esta é %s); atualize a imagem para recebê-la",
			skipped.Version, skipped.MinRuntime, l.cfg.RuntimeVersion)
	}
}

// install baixa, confere sha256 e roda `<bin> --version`. Binário que não
// imprime a versão esperada vai para bad (o índice assinado aponta para um
// artefato errado; tentar de novo não resolve).
func (l *launcher) install(ctx context.Context, v release.Version) error {
	if err := l.fetch.downloadArtifact(ctx, l.store, v, l.cfg.Platform); err != nil {
		return err
	}
	if err := checkBinaryVersion(ctx, l.store.binPath(v.Version), v.Version, l.childEnv); err != nil {
		l.markBad(v.Version)
		return err
	}
	return nil
}

func (l *launcher) markBad(v string) {
	if err := l.store.addBad(v); err != nil {
		l.log.Printf("gravar %s em bad: %v", v, err)
	}
	if v != l.current {
		if err := l.store.removeVersion(v); err != nil {
			l.log.Printf("apagar versão %s: %v", v, err)
		}
	}
}

// checkBinaryVersion roda `<bin> --version` e exige a saída want.
func checkBinaryVersion(ctx context.Context, bin, want string, env []string) error {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s --version: %w", bin, err)
	}
	if got := strings.TrimSpace(out.String()); got != want {
		return fmt.Errorf("%s --version imprimiu %q, esperado %q", bin, got, want)
	}
	return nil
}

// swap troca a versão ativa por target, com rollback.
func (l *launcher) swap(ctx context.Context, target release.Version) {
	from, to := l.current, target.Version
	l.log.Printf("atualizando server-channel de %s para %s", from, to)
	if err := l.install(ctx, target); err != nil {
		l.log.Printf("preparar %s falhou, continua em %s: %v", to, from, err)
		return
	}

	l.stopChild()
	if ctx.Err() != nil {
		return
	}
	l.start(to)
	err := l.waitHealthy(ctx, to)
	if ctx.Err() != nil {
		return // encerrando; current não muda e o próximo boot sobe from
	}
	if err == nil {
		l.current = to
		l.crashes.reset()
		l.crashesSinceUpdate = 0
		if err := l.store.writeLastUpdate(lastUpdate{From: from, To: to, At: time.Now().UTC()}); err != nil {
			l.log.Printf("gravar last-update: %v", err)
		}
		if err := l.store.writeCurrent(to); err != nil {
			l.log.Printf("gravar current: %v", err)
		}
		l.log.Printf("troca concluída: %s → %s", from, to)
		if removed, err := l.store.prune(to, from); err != nil {
			l.log.Printf("podar versions/: %v", err)
		} else if len(removed) > 0 {
			l.log.Printf("versões antigas apagadas: %s", strings.Join(removed, ", "))
		}
		return
	}

	l.log.Printf("versão %s não ficou saudável: %v; rollback para %s", to, err, from)
	l.stopChild()
	l.markBad(to)
	l.start(from)
	if err := l.waitHealthy(ctx, from); err != nil {
		if ctx.Err() == nil {
			l.log.Printf("rollback: %s também não respondeu /healthz: %v", from, err)
		}
		return
	}
	l.log.Printf("rollback concluído: de volta para %s (%s marcada em bad)", from, to)
}

// start sobe o binário de v como filho. Falha em Start vira um filho já
// terminado, tratado pelo laço como saída inesperada.
func (l *launcher) start(v string) {
	cmd := exec.Command(l.store.binPath(v))
	cmd.Env = l.childEnv
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = childSysProcAttr()
	c := &child{cmd: cmd, version: v, started: time.Now(), done: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		c.err = err
		close(c.done)
		l.log.Printf("subir server-channel %s: %v", v, err)
	} else {
		l.log.Printf("server-channel %s iniciado (pid %d)", v, cmd.Process.Pid)
		go func() {
			c.err = cmd.Wait()
			close(c.done)
		}()
	}
	l.child = c
}

// stopChild manda SIGTERM, espera até stopTimeout e mata se preciso.
func (l *launcher) stopChild() {
	c := l.child
	if c == nil {
		return
	}
	select {
	case <-c.done:
		return
	default:
	}
	l.log.Printf("parando server-channel %s (pid %d)", c.version, c.cmd.Process.Pid)
	if err := terminate(c.cmd.Process); err != nil {
		l.log.Printf("SIGTERM: %v", err)
	}
	select {
	case <-c.done:
	case <-time.After(l.stopTimeout):
		l.log.Printf("server-channel %s não saiu em %s; SIGKILL", c.version, l.stopTimeout)
		_ = c.cmd.Process.Kill()
		<-c.done
	}
}

// onChildExit trata a saída inesperada do filho: reinicia com backoff ou,
// depois de maxCrashes saídas seguidas, desiste com código 1.
func (l *launcher) onChildExit(ctx context.Context) (code int, exit bool) {
	c := l.child
	uptime := time.Since(c.started)
	l.log.Printf("server-channel %s saiu sozinho depois de %s: %v", c.version, uptime.Round(time.Millisecond), c.err)
	delay, giveUp := l.crashes.record(uptime)
	l.crashesSinceUpdate++
	if giveUp || l.crashesSinceUpdate >= l.crashes.max {
		if l.rollbackAfterCrashes() {
			l.start(l.current)
			return 0, false
		}
	}
	if giveUp {
		l.log.Printf("server-channel saiu %d vezes seguidas; saindo para o Docker reiniciar o container", l.crashes.max)
		return 1, true
	}
	l.log.Printf("reiniciando server-channel %s em %s", l.current, delay)
	select {
	case <-ctx.Done():
		return 0, false
	case <-time.After(delay):
	}
	l.start(l.current)
	return 0, false
}

// rollbackAfterCrashes volta para a versão anterior quando a atual chegou
// por auto-update há menos de rollbackWindow e está caindo repetidamente,
// mesmo tendo passado no /healthz na troca (ex. panic numa rota específica).
// Dispara com maxCrashes saídas seguidas rápidas (crashPolicy) ou com
// maxCrashes saídas desde a troca, qualquer que seja o uptime de cada uma,
// para uma versão que cai a cada poucos minutos não prender o servidor.
//
// Não age se a versão atual veio da semente ou de troca antiga (sem
// last-update correspondente), se a anterior não está íntegra no disco, ou
// se a atual é o pin exato do operador (FFCOM_CHANNEL_VERSION=X.Y.Z). O
// last-update é apagado no rollback: se a anterior também entrar em crash
// loop, o comportamento volta a ser sair com 1.
func (l *launcher) rollbackAfterCrashes() bool {
	cur := l.current
	lu, err := l.store.readLastUpdate()
	if err != nil {
		l.log.Printf("ler last-update: %v", err)
		return false
	}
	if lu == nil || lu.To != cur || lu.From == "" || time.Since(lu.At) > rollbackWindow {
		return false
	}
	if l.track == cur {
		l.log.Printf("versão %s está caindo repetidamente, mas é o pin de FFCOM_CHANNEL_VERSION; sem rollback automático", cur)
		return false
	}
	if err := l.store.verifyInstalled(lu.From); err != nil {
		l.log.Printf("versão %s está caindo repetidamente, mas a anterior %s não está íntegra para rollback: %v", cur, lu.From, err)
		return false
	}
	l.log.Printf("ROLLBACK: versão %s (instalada em %s) caiu %d vezes depois de passar no /healthz; voltando para %s e marcando %s em bad",
		cur, lu.At.Format(time.RFC3339), l.crashesSinceUpdate, lu.From, cur)
	if err := l.store.writeCurrent(lu.From); err != nil {
		l.log.Printf("gravar current: %v", err)
		return false
	}
	l.current = lu.From
	l.markBad(cur)
	if err := l.store.clearLastUpdate(); err != nil {
		l.log.Printf("apagar last-update: %v", err)
	}
	l.crashes.reset()
	l.crashesSinceUpdate = 0
	return true
}

// crashPolicy conta saídas seguidas do filho. Um filho que ficou no ar por
// stableAfter zera a contagem.
type crashPolicy struct {
	count       int
	max         int
	stableAfter time.Duration
	base        time.Duration
	maxDelay    time.Duration
}

func (p *crashPolicy) reset() { p.count = 0 }

func (p *crashPolicy) record(uptime time.Duration) (delay time.Duration, giveUp bool) {
	if uptime >= p.stableAfter {
		p.count = 0
	}
	p.count++
	if p.count >= p.max {
		return 0, true
	}
	delay = p.base << (p.count - 1)
	if delay > p.maxDelay {
		delay = p.maxDelay
	}
	return delay, false
}

// waitHealthy espera GET /healthz 200 com "version" == want, até
// healthTimeout. Falha cedo se o filho sair.
func (l *launcher) waitHealthy(ctx context.Context, want string) error {
	deadline := time.NewTimer(l.healthTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	lastErr := errors.New("sem resposta")
	for {
		ok, err := l.probe(ctx, want)
		if ok {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.child.done:
			return fmt.Errorf("processo saiu: %v", l.child.err)
		case <-deadline.C:
			return fmt.Errorf("sem /healthz com versão %s em %s (último erro: %v)", want, l.healthTimeout, lastErr)
		case <-tick.C:
		}
	}
}

func (l *launcher) probe(ctx context.Context, want string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.cfg.HealthURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := l.health.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return false, err
	}
	if body.Version != want {
		return false, fmt.Errorf("versão %q no /healthz", body.Version)
	}
	return true, nil
}
