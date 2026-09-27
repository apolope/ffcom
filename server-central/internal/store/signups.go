package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status de um pedido de cadastro (coluna signup_requests.status, ver
// migration 0011_signup_requests).
const (
	SignupPending   = "pending"
	SignupApproving = "approving"
	SignupApproved  = "approved"
	SignupRejected  = "rejected"
)

// signupApprovalStale é quanto tempo um pedido pode ficar em approving antes
// de um clique novo poder retomá-lo (o processo morreu no meio da
// aprovação). As chamadas ao Authentik levam segundos.
const signupApprovalStale = 5 * time.Minute

// SignupRequest é um pedido de cadastro feito pela home page.
type SignupRequest struct {
	ID       string
	FullName string
	// Username e Nickname ficam vazios quando ExistingAccount: a pessoa já
	// tem conta no Authentik e a aprovação só a põe no grupo.
	Username        string
	Email           string
	Nickname        string
	Reason          string
	ExistingAccount bool
	// EmailAccounts são as contas do Authentik com o e-mail do pedido,
	// conferidas no envio (ver migration 0012); nil se não deu para
	// conferir.
	EmailAccounts     *string
	Status            string
	ClientIP          string
	TelegramMessageID *int64
	DecidedBy         *string
	DecidedAt         *time.Time
	Failure           *string
	AuthentikUserPK   *int64
	AuthentikUsername *string
	// LinkedExisting diz que a aprovação usou uma conta que já existia.
	LinkedExisting bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewSignup são os campos que a pessoa preenche no formulário, já
// validados e normalizados pelo handler.
type NewSignup struct {
	FullName        string
	Username        string
	Email           string
	Nickname        string
	Reason          string
	ExistingAccount bool
	EmailAccounts   *string
	ClientIP        string
}

type SignupStore struct {
	pool *pgxpool.Pool
}

const signupColumns = `id, full_name, coalesce(username, ''), email, coalesce(nickname, ''), reason,
	existing_account, email_accounts, status, client_ip, telegram_message_id, decided_by, decided_at,
	failure, authentik_user_pk, authentik_username, linked_existing, created_at, updated_at`

func scanSignup(row pgx.Row) (SignupRequest, error) {
	var s SignupRequest
	err := row.Scan(&s.ID, &s.FullName, &s.Username, &s.Email, &s.Nickname, &s.Reason,
		&s.ExistingAccount, &s.EmailAccounts, &s.Status, &s.ClientIP, &s.TelegramMessageID, &s.DecidedBy, &s.DecidedAt,
		&s.Failure, &s.AuthentikUserPK, &s.AuthentikUsername, &s.LinkedExisting, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignupRequest{}, ErrNotFound
	}
	return s, err
}

// Create grava um pedido novo. ErrConflict quando já há um pedido em
// aberto com o mesmo e-mail ou nome de usuário.
func (s *SignupStore) Create(ctx context.Context, n NewSignup) (SignupRequest, error) {
	query := `
		INSERT INTO signup_requests (full_name, username, email, nickname, reason, existing_account, email_accounts, client_ip)
		VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, ''), $5, $6, $7, $8)
		RETURNING ` + signupColumns
	req, err := scanSignup(s.pool.QueryRow(ctx, query, n.FullName, n.Username, n.Email, n.Nickname, n.Reason,
		n.ExistingAccount, n.EmailAccounts, n.ClientIP))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return SignupRequest{}, ErrConflict
	}
	if err != nil {
		return SignupRequest{}, fmt.Errorf("signups: criar: %w", err)
	}
	return req, nil
}

// Get devolve o pedido id (ErrNotFound se não existir).
func (s *SignupStore) Get(ctx context.Context, id string) (SignupRequest, error) {
	req, err := scanSignup(s.pool.QueryRow(ctx, `SELECT `+signupColumns+` FROM signup_requests WHERE id = $1`, id))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SignupRequest{}, fmt.Errorf("signups: get: %w", err)
	}
	return req, err
}

// OpenOrApprovedFor devolve o pedido em aberto ou aprovado mais recente com
// o e-mail ou o nome de usuário informados (ErrNotFound se não houver).
// Reprovados não contam: a pessoa pode pedir de novo. username vazio (quem
// já tem conta) só confere o e-mail.
func (s *SignupStore) OpenOrApprovedFor(ctx context.Context, email, username string) (SignupRequest, error) {
	query := `SELECT ` + signupColumns + ` FROM signup_requests
		WHERE (email = $1 OR username = $2) AND status <> 'rejected'
		ORDER BY created_at DESC LIMIT 1`
	req, err := scanSignup(s.pool.QueryRow(ctx, query, email, username))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SignupRequest{}, fmt.Errorf("signups: buscar por e-mail/usuário: %w", err)
	}
	return req, err
}

// CountSince conta os pedidos feitos desde since, todos (clientIP vazio) ou
// só os vindos de clientIP.
func (s *SignupStore) CountSince(ctx context.Context, clientIP string, since time.Time) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM signup_requests
		WHERE created_at >= $1 AND ($2 = '' OR client_ip = $2)`, since, clientIP).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("signups: contar: %w", err)
	}
	return n, nil
}

// SetTelegramMessage guarda o id da mensagem enviada ao grupo, usada depois
// para editar a mensagem com a decisão.
func (s *SignupStore) SetTelegramMessage(ctx context.Context, id string, messageID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE signup_requests SET telegram_message_id = $2 WHERE id = $1`, id, messageID)
	if err != nil {
		return fmt.Errorf("signups: gravar mensagem do telegram: %w", err)
	}
	return nil
}

// ListUnsent devolve pedidos em aberto, criados antes de before, cujo envio
// ao Telegram falhou, dos mais antigos para os mais novos. before deixa de
// fora os que o handler ainda pode estar enviando.
func (s *SignupStore) ListUnsent(ctx context.Context, before time.Time, limit int) ([]SignupRequest, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+signupColumns+` FROM signup_requests
		WHERE telegram_message_id IS NULL AND status = 'pending' AND created_at < $1
		ORDER BY created_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("signups: listar sem mensagem: %w", err)
	}
	defer rows.Close()
	var out []SignupRequest
	for rows.Next() {
		req, err := scanSignup(rows)
		if err != nil {
			return nil, fmt.Errorf("signups: scan: %w", err)
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

// ClaimApproval passa o pedido para approving, em nome de decidedBy, se ele
// estiver pendente (ou preso em approving há mais de alguns minutos).
// ErrNotFound quando o pedido não existe ou já foi decidido: quem chama
// relê com Get para contar o que aconteceu. Um clique só avança; dois
// cliques simultâneos em Aprovar não criam o usuário duas vezes.
func (s *SignupStore) ClaimApproval(ctx context.Context, id, decidedBy string) (SignupRequest, error) {
	query := `
		UPDATE signup_requests
		SET status = 'approving', decided_by = $2, updated_at = now()
		WHERE id = $1 AND (status = 'pending' OR (status = 'approving' AND updated_at < now() - $3::interval))
		RETURNING ` + signupColumns
	req, err := scanSignup(s.pool.QueryRow(ctx, query, id, decidedBy, fmt.Sprintf("%d seconds", int(signupApprovalStale.Seconds()))))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SignupRequest{}, fmt.Errorf("signups: reservar aprovação: %w", err)
	}
	return req, err
}

// ReleaseApproval devolve a pendente um pedido cuja aprovação falhou,
// guardando o motivo, para alguém clicar de novo ou reprovar.
func (s *SignupStore) ReleaseApproval(ctx context.Context, id, failure string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE signup_requests SET status = 'pending', decided_by = NULL, failure = $2, updated_at = now()
		WHERE id = $1 AND status = 'approving'`, id, failure)
	if err != nil {
		return fmt.Errorf("signups: liberar aprovação: %w", err)
	}
	return nil
}

// FinishApproval marca o pedido como aprovado, com o usuário do Authentik
// criado ou, se linkedExisting, o que já existia e só entrou no grupo.
// warning guarda o que não saiu como esperado sem desfazer a aprovação (ex.
// o e-mail de senha falhou), ou nil.
func (s *SignupStore) FinishApproval(ctx context.Context, id string, authentikUserPK int64, authentikUsername string, linkedExisting bool, warning *string) (SignupRequest, error) {
	query := `
		UPDATE signup_requests
		SET status = 'approved', authentik_user_pk = $2, authentik_username = $3, linked_existing = $4,
			failure = $5, decided_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'approving'
		RETURNING ` + signupColumns
	req, err := scanSignup(s.pool.QueryRow(ctx, query, id, authentikUserPK, authentikUsername, linkedExisting, warning))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SignupRequest{}, fmt.Errorf("signups: concluir aprovação: %w", err)
	}
	return req, err
}

// Reject reprova um pedido pendente. ErrNotFound quando o pedido não existe
// ou não está mais pendente.
func (s *SignupStore) Reject(ctx context.Context, id, decidedBy string) (SignupRequest, error) {
	query := `
		UPDATE signup_requests
		SET status = 'rejected', decided_by = $2, decided_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING ` + signupColumns
	req, err := scanSignup(s.pool.QueryRow(ctx, query, id, decidedBy))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SignupRequest{}, fmt.Errorf("signups: reprovar: %w", err)
	}
	return req, err
}
