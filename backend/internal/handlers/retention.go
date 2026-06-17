package handlers

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartRetentionWorker(ctx context.Context, db *pgxpool.Pool) {
	// Run once at startup, then every 24 hours
	go runRetention(ctx, db)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runRetention(ctx, db)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func runRetention(ctx context.Context, db *pgxpool.Pool) {
	msgDays := settingInt(ctx, db, "message_retention_days")
	spaceDays := settingInt(ctx, db, "inactive_space_retention_days")

	if msgDays > 0 {
		tag, err := db.Exec(ctx, `
			DELETE FROM messages WHERE created_at < NOW() - ($1 || ' days')::INTERVAL
		`, strconv.Itoa(msgDays))
		if err != nil {
			log.Printf("retention: delete messages: %v", err)
		} else {
			log.Printf("retention: deleted %d channel messages older than %d days", tag.RowsAffected(), msgDays)
		}

		tag, err = db.Exec(ctx, `
			DELETE FROM direct_messages WHERE created_at < NOW() - ($1 || ' days')::INTERVAL
		`, strconv.Itoa(msgDays))
		if err != nil {
			log.Printf("retention: delete direct_messages: %v", err)
		} else {
			log.Printf("retention: deleted %d direct messages older than %d days", tag.RowsAffected(), msgDays)
		}
	}

	if spaceDays > 0 {
		// Delete spaces where no message has been sent in spaceDays days
		// (and the space itself is at least that old, so new empty spaces aren't purged)
		tag, err := db.Exec(ctx, `
			DELETE FROM servers
			WHERE created_at < NOW() - ($1 || ' days')::INTERVAL
			AND id NOT IN (
				SELECT DISTINCT c.server_id
				FROM channels c
				JOIN messages m ON m.channel_id = c.id
				WHERE m.created_at > NOW() - ($1 || ' days')::INTERVAL
			)
		`, strconv.Itoa(spaceDays))
		if err != nil {
			log.Printf("retention: delete inactive spaces: %v", err)
		} else {
			log.Printf("retention: deleted %d inactive spaces (no activity in %d days)", tag.RowsAffected(), spaceDays)
		}
	}
}

func settingInt(ctx context.Context, db *pgxpool.Pool, key string) int {
	var val string
	db.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&val)
	n, _ := strconv.Atoi(val)
	return n
}
