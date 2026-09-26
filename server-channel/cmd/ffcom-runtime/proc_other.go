//go:build !unix

package main

// Fora de unix (Windows do dev) o lançador compila e os testes da lógica
// portável rodam, mas não há SIGTERM nem SIGHUP: parar o filho é matar o
// processo, e a checagem só acontece pelo intervalo. O alvo real é linux.

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func childSysProcAttr() *syscall.SysProcAttr { return nil }

func terminate(p *os.Process) error { return p.Kill() }

func notifySignals(logger *log.Logger, cancel func(), _ chan<- struct{}) (stop func()) {
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case s := <-term:
			logger.Printf("recebeu %v; parando server-channel e saindo", s)
			cancel()
		case <-done:
		}
	}()
	return func() {
		signal.Stop(term)
		close(done)
	}
}
