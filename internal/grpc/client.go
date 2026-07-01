package grpc

import (
	"clinithink/pb" // Hasil generate proto
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func SendAnalysisTrigger(sessionID string, input string, title string, diagnosis string, workup []string, anamnesis []string) (*pb.AnalysisResponse, error) {
	conn, err := grpc.NewClient("ai:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewClinicServiceClient(conn)

	// Tangkap return value 'res'
	res, err := client.TriggerAnalysis(context.Background(), &pb.AnalysisRequest{
		SessionId:        sessionID,
		UserInput:        input,
		Title:            title,
		PrimaryDiagnosis: diagnosis,
		ExpectedWorkup:   workup,
		AnamnesisItems:   anamnesis,
	})

	if err != nil {
		return nil, err
	}

	return res, nil // Kembalikan respon dari AI
}
