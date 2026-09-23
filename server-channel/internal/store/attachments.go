package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AttachmentStore struct {
	pool *pgxpool.Pool
}

// Create grava o registro de um anexo cujo arquivo já foi salvo em disco
// (ver internal/storage.FileStore), associado a uma mensagem já existente.
func (s *AttachmentStore) Create(ctx context.Context, messageID, filename, contentType string, sizeBytes int64, storageKey string) (Attachment, error) {
	const query = `
		INSERT INTO attachments (message_id, filename, content_type, size_bytes, storage_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, message_id, filename, content_type, size_bytes, storage_key, created_at
	`
	var a Attachment
	err := s.pool.QueryRow(ctx, query, messageID, filename, contentType, sizeBytes, storageKey).
		Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt)
	if err != nil {
		return Attachment{}, fmt.Errorf("attachments: create: %w", err)
	}
	return a, nil
}

// GetByID busca um anexo por id -- usado por GET /api/attachments/{id} para
// resolver a mensagem (e portanto o canal) antes de checar ViewChannels.
func (s *AttachmentStore) GetByID(ctx context.Context, id string) (Attachment, error) {
	const query = `
		SELECT id, message_id, filename, content_type, size_bytes, storage_key, created_at
		FROM attachments
		WHERE id = $1
	`
	var a Attachment
	err := s.pool.QueryRow(ctx, query, id).
		Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("attachments: get by id: %w", err)
	}
	return a, nil
}

// ListForMessages devolve os anexos de várias mensagens de uma vez (usado
// pelo histórico paginado, ver handleListMessages), agrupados por
// message_id -- mesmo padrão de RoleStore.AssignmentsForMembers, evita N+1.
func (s *AttachmentStore) ListForMessages(ctx context.Context, messageIDs []string) (map[string][]Attachment, error) {
	out := make(map[string][]Attachment, len(messageIDs))
	if len(messageIDs) == 0 {
		return out, nil
	}

	const query = `
		SELECT id, message_id, filename, content_type, size_bytes, storage_key, created_at
		FROM attachments
		WHERE message_id = ANY($1)
		ORDER BY created_at
	`
	rows, err := s.pool.Query(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("attachments: list para mensagens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("attachments: scan: %w", err)
		}
		out[a.MessageID] = append(out[a.MessageID], a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("attachments: iterar linhas: %w", err)
	}
	return out, nil
}

// ListForMessage é um atalho de ListForMessages para uma única mensagem --
// usado ao editar (reanexar no broadcast, ver internal/httpapi/channel_ws.go)
// ou apagar (recuperar storage_key antes do DELETE em cascata) uma mensagem
// via WebSocket.
func (s *AttachmentStore) ListForMessage(ctx context.Context, messageID string) ([]Attachment, error) {
	byMessage, err := s.ListForMessages(ctx, []string{messageID})
	if err != nil {
		return nil, err
	}
	return byMessage[messageID], nil
}

// StorageKeysForChannel devolve a chave em disco de todo anexo de mensagem
// do canal -- usado antes de apagar o canal inteiro, já que o CASCADE do
// Postgres apaga as linhas mas não os arquivos (ver ChannelStore.Delete).
func (s *AttachmentStore) StorageKeysForChannel(ctx context.Context, channelID string) ([]string, error) {
	const query = `
		SELECT a.storage_key
		FROM attachments a
		JOIN messages m ON m.id = a.message_id
		WHERE m.channel_id = $1
	`
	rows, err := s.pool.Query(ctx, query, channelID)
	if err != nil {
		return nil, fmt.Errorf("attachments: storage keys por canal: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("attachments: scan: %w", err)
		}
		out = append(out, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("attachments: iterar linhas: %w", err)
	}
	return out, nil
}
