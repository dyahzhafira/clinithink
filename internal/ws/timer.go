package ws

import (
	"context"
	"encoding/json"
	"log"
	"time"

	livekit_proto "github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// StartTimer mengirim timer_tick ke semua peserta room LiveKit setiap detik.
// ctx digunakan untuk membatalkan timer (misalnya saat sesi disubmit).
func StartTimer(ctx context.Context, sessionID string, remainingSec int, roomClient *lksdk.RoomServiceClient) {
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
				log.Printf("[Timer] Waktu sesi %s habis.", sessionID)
				return
			}
		}
	}
}
