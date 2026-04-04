package server

import (
	"net"

	"refina-transaction/config/db"
	"refina-transaction/config/env"
	"refina-transaction/config/miniofs"
	grpcclient "refina-transaction/interface/grpc/client"
	"refina-transaction/interface/grpc/interceptor"
	queueclient "refina-transaction/interface/queue/client"
	"refina-transaction/internal/repository"
	"refina-transaction/internal/service"

	tpb "github.com/MuhammadMiftaa/Refina-Protobuf/transaction"
	"google.golang.org/grpc"
)

func SetupGRPCServer(dbInstance db.DatabaseClient, minioInstance *miniofs.MinIOManager, queueInstance queueclient.RabbitMQClient) (*grpc.Server, *net.Listener, error) {
	lis, err := net.Listen("tcp", ":"+env.Cfg.Server.GRPCPort)
	if err != nil {
		return nil, nil, err
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.UnaryServerInterceptor()),
		grpc.StreamInterceptor(interceptor.StreamServerInterceptor()),
	)

	// ── Repositories ──
	txManager := repository.NewTxManager(dbInstance.GetDB())
	transactionsRepo := repository.NewTransactionRepository(dbInstance.GetDB())
	categoryRepo := repository.NewCategoryRepository(dbInstance.GetDB())
	attachmentRepo := repository.NewAttachmentsRepository(dbInstance.GetDB())
	outboxRepo := repository.NewOutboxRepository(dbInstance.GetDB())
	budgetsRepo := repository.NewBudgetsRepository(dbInstance.GetDB())

	// ── gRPC Client (wallet) ──
	walletClient := grpcclient.NewWalletClient(grpcclient.GetManager().GetWalletClient())

	// ── Services ──
	transactionService := service.NewTransactionService(
		txManager,
		transactionsRepo,
		walletClient,
		categoryRepo,
		attachmentRepo,
		outboxRepo,
		minioInstance,
	)
	categoryService := service.NewCategoriesService(txManager, categoryRepo)
	attachmentService := service.NewAttachmentsService(txManager, attachmentRepo)
	budgetsService := service.NewBudgetsService(txManager, budgetsRepo)

	txnServer := &transactionServer{
		transactionService: transactionService,
		categoryService:    categoryService,
		attachmentService:  attachmentService,
		budgetsService:     budgetsService,
	}
	tpb.RegisterTransactionServiceServer(s, txnServer)

	return s, &lis, nil
}
