package ws

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartTimer(ctx context.Context, sessionID string, remainingSec int, db *pgxpool.Pool, hub *Hub) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			remainingSec--
			hub.Send(sessionID, Event{
				Type:    "timer_tick",
				Payload: map[string]int{"seconds_remaining": remainingSec},
			})
			if remainingSec <= 0 {
				db.Exec(context.Background(),
					`UPDATE sessions SET status = 'abandoned', submitted_at = now()
					 WHERE id = $1 AND status = 'in_progress'`,
					sessionID,
				)
				hub.Send(sessionID, Event{
					Type:    "session_ended",
					Payload: map[string]string{"reason": "timeout"},
				})
				return
			}
		}
	}
}
