package grpc

import (
	"clinithink/pb" // Hasil generate proto
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func SendAnalysisTrigger(sessionID string, input string, title string, diagnosis string, workup []string, anamnesis []string) error {
	conn, err := grpc.Dial("ai:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewClinicServiceClient(conn)

	_, err = client.TriggerAnalysis(context.Background(), &pb.AnalysisRequest{
		SessionId:        sessionID,
		UserInput:        input,
		Title:            title,
		PrimaryDiagnosis: diagnosis,
		ExpectedWorkup:   workup,
		AnamnesisItems:   anamnesis, // Field ini sesuai dengan hasil generate pb
	})
	return err
}
