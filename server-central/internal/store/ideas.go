package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status de uma ideia (coluna ideas.status, ver migration 0009_ideas).
const (
	IdeaChecking    = "checking"
	IdeaReview      = "review"
	IdeaOpen        = "open"
	IdeaPlanned     = "planned"
	IdeaImplemented = "implemented"
	IdeaRejected    = "rejected"
)

// Tipos e status de pedidos ao a3s-claude-relay (tabela idea_assist_jobs).
const (
	AssistImprove = "improve"
	AssistCheck   = "check"

	AssistPending = "pending"
	AssistDone    = "done"
	AssistFailed  = "failed"
)

// Pesos da pontuação de uma ideia: like vale 2, dislike tira 1.
const (
	IdeaLikeWeight    = 2
	IdeaDislikeWeight = 1
)

// Idea é uma sugestão de melhoria, já com os votos agregados e o voto de
// quem está vendo (MyVote: 1, -1 ou 0).
type Idea struct {
	ID                 string
	AccountID          string
	AuthorFirstName    string
	Title              string
	Body               string
	Status             string
	ReviewReason       *string
	ImplementedVersion *string
	SuggestedOn        time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Likes              int
	Dislikes           int
	MyVote             int
}

// Score é a pontuação exibida e usada na ordenação do ranking.
func (i Idea) Score() int {
	return i.Likes*IdeaLikeWeight - i.Dislikes*IdeaDislikeWeight
}

// AssistJob é um pedido ao a3s-claude-relay, da varinha (improve) ou da
// checagem final de uma ideia enviada (check).
type AssistJob struct {
	ID         string
	Kind       string
	AccountID  string
	Day        time.Time
	IdeaID     *string
	Status     string
	RelayJobID *string
	Result     []byte
	CreatedAt  time.Time
}

type IdeaStore struct {
	pool *pgxpool.Pool
}

// ideaSelect traz a ideia com o primeiro nome do autor (mesmo nome exibido
// no resto do FFCom, ver displayNameSQL) e os votos agregados. $1 é a conta
// de quem está vendo, ou NULL, para MyVote.
const ideaSelect = `
	SELECT i.id, i.account_id,
	       split_part(btrim(COALESCE(` + displayNameSQL + `, '')), ' ', 1),
	       i.title, i.body, i.status, i.review_reason, i.implemented_version,
	       i.suggested_on, i.created_at, i.updated_at,
	       COALESCE(v.likes, 0), COALESCE(v.dislikes, 0), COALESCE(mv.value, 0)
	FROM ideas i
	JOIN accounts a ON a.id = i.account_id
	LEFT JOIN profiles p ON p.account_id = a.id
	LEFT JOIN (
		SELECT idea_id,
		       count(*) FILTER (WHERE value = 1) AS likes,
		       count(*) FILTER (WHERE value = -1) AS dislikes
		FROM idea_votes GROUP BY idea_id
	) v ON v.idea_id = i.id
	LEFT JOIN idea_votes mv ON mv.idea_id = i.id AND mv.account_id = $1::uuid
`

// IdeaOrder escolhe a ordenação de List.
type IdeaOrder int

const (
	// IdeaOrderScore: maior pontuação primeiro, empate para a mais antiga.
	IdeaOrderScore IdeaOrder = iota
	// IdeaOrderRecent: atualizada mais recentemente primeiro (implementadas).
	IdeaOrderRecent
	// IdeaOrderOldest: mais antiga primeiro (fila de moderação).
	IdeaOrderOldest
)

// List devolve as ideias nos status pedidos. viewerID vazio = visitante sem
// login (MyVote sempre 0).
func (s *IdeaStore) List(ctx context.Context, statuses []string, viewerID string, order IdeaOrder, limit int) ([]Idea, error) {
	orderBy := fmt.Sprintf("(COALESCE(v.likes, 0) * %d - COALESCE(v.dislikes, 0) * %d) DESC, i.created_at ASC", IdeaLikeWeight, IdeaDislikeWeight)
	switch order {
	case IdeaOrderRecent:
		orderBy = "i.updated_at DESC"
	case IdeaOrderOldest:
		orderBy = "i.created_at ASC"
	}
	query := ideaSelect + ` WHERE i.status = ANY($2) ORDER BY ` + orderBy + ` LIMIT $3`
	return s.scanMany(ctx, query, nullableID(viewerID), statuses, limit)
}

// Get devolve uma ideia pelo id.
func (s *IdeaStore) Get(ctx context.Context, id, viewerID string) (Idea, error) {
	ideas, err := s.scanMany(ctx, ideaSelect+` WHERE i.id = $2`, nullableID(viewerID), id)
	if err != nil {
		return Idea{}, err
	}
	if len(ideas) == 0 {
		return Idea{}, ErrNotFound
	}
	return ideas[0], nil
}

// GetMany devolve as ideias dos ids pedidos que estejam nos status
// informados, na ordem do ranking.
func (s *IdeaStore) GetMany(ctx context.Context, ids, statuses []string, viewerID string) ([]Idea, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := ideaSelect + fmt.Sprintf(` WHERE i.id = ANY($2::uuid[]) AND i.status = ANY($3)
		ORDER BY (COALESCE(v.likes, 0) * %d - COALESCE(v.dislikes, 0) * %d) DESC, i.created_at ASC`, IdeaLikeWeight, IdeaDislikeWeight)
	return s.scanMany(ctx, query, nullableID(viewerID), ids, statuses)
}

// ForDay devolve a ideia que accountID enviou no dia day (ErrNotFound se
// ainda não enviou).
func (s *IdeaStore) ForDay(ctx context.Context, accountID string, day time.Time) (Idea, error) {
	ideas, err := s.scanMany(ctx, ideaSelect+` WHERE i.account_id = $2 AND i.suggested_on = $3`, accountID, accountID, day)
	if err != nil {
		return Idea{}, err
	}
	if len(ideas) == 0 {
		return Idea{}, ErrNotFound
	}
	return ideas[0], nil
}

// Create grava a ideia do dia em status checking. ErrConflict se a pessoa
// já enviou uma no mesmo dia (índice ideas_one_per_day).
func (s *IdeaStore) Create(ctx context.Context, accountID, body string, day time.Time) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO ideas (account_id, body, suggested_on) VALUES ($1, $2, $3) RETURNING id`,
		accountID, body, day).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return "", ErrConflict
	}
	if err != nil {
		return "", fmt.Errorf("ideas: criar: %w", err)
	}
	return id, nil
}

// FinishCheck aplica o resultado da checagem final, só se a ideia ainda
// estiver em checking (um callback atrasado depois do prazo não muda
// nada). title vazio mantém o atual; body nil mantém o texto enviado.
func (s *IdeaStore) FinishCheck(ctx context.Context, id, status, title string, body, reason *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE ideas
		SET status = $2,
		    title = CASE WHEN $3 = '' THEN title ELSE $3 END,
		    body = COALESCE($4, body),
		    review_reason = $5,
		    updated_at = now()
		WHERE id = $1 AND status = 'checking'`,
		id, status, title, body, reason)
	if err != nil {
		return fmt.Errorf("ideas: aplicar checagem: %w", err)
	}
	return nil
}

// SetStatus é a ação do admin: aprovar (open), planejar, marcar como
// implementada (com a versão) ou recusar. Sair de review limpa o motivo.
func (s *IdeaStore) SetStatus(ctx context.Context, id, status string, version *string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ideas
		SET status = $2,
		    implemented_version = CASE WHEN $2 = 'implemented' THEN $3 ELSE NULL END,
		    review_reason = CASE WHEN $2 = 'review' THEN review_reason ELSE NULL END,
		    title = CASE WHEN title = '' THEN left(body, 60) ELSE title END,
		    updated_at = now()
		WHERE id = $1`,
		id, status, version)
	if err != nil {
		return fmt.Errorf("ideas: mudar status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Vote grava o voto de accountID (1 like, -1 dislike, 0 tira o voto).
func (s *IdeaStore) Vote(ctx context.Context, ideaID, accountID string, value int) error {
	var err error
	if value == 0 {
		_, err = s.pool.Exec(ctx, `DELETE FROM idea_votes WHERE idea_id = $1 AND account_id = $2`, ideaID, accountID)
	} else {
		_, err = s.pool.Exec(ctx, `
			INSERT INTO idea_votes (idea_id, account_id, value) VALUES ($1, $2, $3)
			ON CONFLICT (idea_id, account_id) DO UPDATE SET value = EXCLUDED.value, created_at = now()`,
			ideaID, accountID, value)
	}
	if err != nil {
		return fmt.Errorf("ideas: votar: %w", err)
	}
	return nil
}

// WandUsage devolve os usos e a penalidade da varinha de accountID no dia.
func (s *IdeaStore) WandUsage(ctx context.Context, accountID string, day time.Time) (used, penalty int, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT wand_used, wand_penalty FROM idea_daily_usage WHERE account_id = $1 AND day = $2`,
		accountID, day).Scan(&used, &penalty)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("ideas: ler uso da varinha: %w", err)
	}
	return used, penalty, nil
}

// ReserveWand gasta um uso da varinha se ainda houver saldo (usos +
// penalidade abaixo de limit), de forma atômica. false = sem saldo.
func (s *IdeaStore) ReserveWand(ctx context.Context, accountID string, day time.Time, limit int) (bool, error) {
	var used int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO idea_daily_usage (account_id, day, wand_used) VALUES ($1, $2, 1)
		ON CONFLICT (account_id, day) DO UPDATE SET wand_used = idea_daily_usage.wand_used + 1
		WHERE idea_daily_usage.wand_used + idea_daily_usage.wand_penalty < $3
		RETURNING wand_used`,
		accountID, day, limit).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("ideas: reservar uso da varinha: %w", err)
	}
	return true, nil
}

// RefundWand devolve o uso reservado de um pedido que falhou.
func (s *IdeaStore) RefundWand(ctx context.Context, accountID string, day time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE idea_daily_usage SET wand_used = GREATEST(wand_used - 1, 0) WHERE account_id = $1 AND day = $2`,
		accountID, day)
	if err != nil {
		return fmt.Errorf("ideas: devolver uso da varinha: %w", err)
	}
	return nil
}

// PenalizeWand tira amount do saldo da varinha no dia (ideia ofensiva).
func (s *IdeaStore) PenalizeWand(ctx context.Context, accountID string, day time.Time, amount int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO idea_daily_usage (account_id, day, wand_penalty) VALUES ($1, $2, $3)
		ON CONFLICT (account_id, day) DO UPDATE SET wand_penalty = idea_daily_usage.wand_penalty + $3`,
		accountID, day, amount)
	if err != nil {
		return fmt.Errorf("ideas: penalizar varinha: %w", err)
	}
	return nil
}

// CreateJob registra um pedido ao relay antes de enviá-lo (o id vai na URL
// do callback, então precisa existir antes da resposta chegar). result
// guarda, enquanto o pedido está pendente, o que o callback precisa saber
// (na varinha, os ids das ideias listadas no prompt, na ordem numerada).
func (s *IdeaStore) CreateJob(ctx context.Context, kind, accountID string, day time.Time, ideaID *string, result []byte) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO idea_assist_jobs (kind, account_id, day, idea_id, result) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		kind, accountID, day, ideaID, result).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("ideas: criar pedido ao relay: %w", err)
	}
	return id, nil
}

// SetRelayJobID guarda o jobId devolvido pelo relay (só para log/depuração).
func (s *IdeaStore) SetRelayJobID(ctx context.Context, id, relayJobID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE idea_assist_jobs SET relay_job_id = $2 WHERE id = $1`, id, relayJobID)
	if err != nil {
		return fmt.Errorf("ideas: gravar jobId do relay: %w", err)
	}
	return nil
}

const jobSelect = `SELECT id, kind, account_id, day, idea_id, status, relay_job_id, result, created_at FROM idea_assist_jobs`

// GetJob devolve um pedido pelo id.
func (s *IdeaStore) GetJob(ctx context.Context, id string) (AssistJob, error) {
	jobs, err := s.scanJobs(ctx, jobSelect+` WHERE id = $1`, id)
	if err != nil {
		return AssistJob{}, err
	}
	if len(jobs) == 0 {
		return AssistJob{}, ErrNotFound
	}
	return jobs[0], nil
}

// PendingJob devolve o pedido pendente de accountID do tipo kind, se houver.
func (s *IdeaStore) PendingJob(ctx context.Context, accountID, kind string) (AssistJob, error) {
	jobs, err := s.scanJobs(ctx, jobSelect+` WHERE account_id = $1 AND kind = $2 AND status = 'pending' ORDER BY created_at DESC LIMIT 1`, accountID, kind)
	if err != nil {
		return AssistJob{}, err
	}
	if len(jobs) == 0 {
		return AssistJob{}, ErrNotFound
	}
	return jobs[0], nil
}

// FinishJob fecha um pedido pendente com status e result. Devolve o pedido
// e true só para quem de fato fechou: callback repetido ou atrasado (depois
// do prazo) recebe false e não deve aplicar nada.
func (s *IdeaStore) FinishJob(ctx context.Context, id, status string, result []byte) (AssistJob, bool, error) {
	jobs, err := s.scanJobs(ctx, `
		UPDATE idea_assist_jobs SET status = $2, result = $3, finished_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING id, kind, account_id, day, idea_id, status, relay_job_id, result, created_at`,
		id, status, result)
	if err != nil {
		return AssistJob{}, false, err
	}
	if len(jobs) == 0 {
		return AssistJob{}, false, nil
	}
	return jobs[0], true, nil
}

// ExpiredJobs devolve os pedidos do tipo kind pendentes há mais de maxAge.
func (s *IdeaStore) ExpiredJobs(ctx context.Context, kind string, maxAge time.Duration) ([]AssistJob, error) {
	return s.scanJobs(ctx, jobSelect+` WHERE kind = $1 AND status = 'pending' AND created_at < now() - $2::interval`,
		kind, fmt.Sprintf("%d seconds", int(maxAge.Seconds())))
}

func (s *IdeaStore) scanMany(ctx context.Context, query string, args ...any) ([]Idea, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ideas: query: %w", err)
	}
	defer rows.Close()
	var ideas []Idea
	for rows.Next() {
		var i Idea
		if err := rows.Scan(&i.ID, &i.AccountID, &i.AuthorFirstName, &i.Title, &i.Body, &i.Status,
			&i.ReviewReason, &i.ImplementedVersion, &i.SuggestedOn, &i.CreatedAt, &i.UpdatedAt,
			&i.Likes, &i.Dislikes, &i.MyVote); err != nil {
			return nil, fmt.Errorf("ideas: ler linha: %w", err)
		}
		if strings.TrimSpace(i.AuthorFirstName) == "" {
			i.AuthorFirstName = "Anônimo"
		}
		ideas = append(ideas, i)
	}
	return ideas, rows.Err()
}

func (s *IdeaStore) scanJobs(ctx context.Context, query string, args ...any) ([]AssistJob, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("idea_assist_jobs: query: %w", err)
	}
	defer rows.Close()
	var jobs []AssistJob
	for rows.Next() {
		var j AssistJob
		if err := rows.Scan(&j.ID, &j.Kind, &j.AccountID, &j.Day, &j.IdeaID, &j.Status, &j.RelayJobID, &j.Result, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("idea_assist_jobs: ler linha: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// nullableID converte "" em NULL para os parâmetros $N::uuid.
func nullableID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}
