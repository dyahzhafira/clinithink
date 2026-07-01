package ws

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	livekit_proto "github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// StartTimer mengirim timer_tick ke semua peserta room LiveKit setiap detik.
// ctx digunakan untuk membatalkan timer (misalnya saat sesi disubmit).
func StartTimer(ctx context.Context, sessionID string, remainingSec int, roomClient *lksdk.RoomServiceClient, db *pgxpool.Pool) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Timer] Timer sesi %s dihentikan.", sessionID)
			return
		case <-ticker.C:
			remainingSec--

			data, _ := json.Marshal(map[string]interface{}{
				"type":    "timer_tick",
				"payload": map[string]int{"seconds_remaining": remainingSec},
			})

			_, err := roomClient.SendData(ctx, &livekit_proto.SendDataRequest{
				Room: sessionID,
				Data: data,
				Kind: livekit_proto.DataPacket_RELIABLE,
			})
			if err != nil {
				log.Printf("[Timer] Gagal kirim timer_tick ke room %s: %v", sessionID, err)
			}

			if remainingSec <= 0 {
				log.Printf("[Timer] Waktu sesi %s habis. Mengupdate status ke abandoned...", sessionID)
				
				// 1. Update session status di DB ke 'abandoned'
				_, err = db.Exec(context.Background(),
					`UPDATE sessions SET status = 'abandoned', submitted_at = now() WHERE id = $1 AND status = 'in_progress'`,
					sessionID,
				)
				if err != nil {
					log.Printf("[Timer] Gagal update status sesi %s ke abandoned: %v", sessionID, err)
				}

				// 2. Broadcast session_ended ke room LiveKit agar klien tahu sesi berakhir
				endData, _ := json.Marshal(map[string]interface{}{
					"type":    "session_ended",
					"payload": map[string]string{"reason": "timeout"},
				})
				_, err = roomClient.SendData(ctx, &livekit_proto.SendDataRequest{
					Room: sessionID,
					Data: endData,
					Kind: livekit_proto.DataPacket_RELIABLE,
				})
				if err != nil {
					log.Printf("[Timer] Gagal kirim session_ended ke room %s: %v", sessionID, err)
				}
				return
			}
		}
	}
}
