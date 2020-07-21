package main

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/joho/godotenv"
	pb "github.com/transavro/SearchService/gen"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	mongoDbHost, grpcPort, restPort string
	currentIndex                    = 0
	youtubeApiKeys                  []string
	currentKey                      string
	err								error
)


func startGRPCServer(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s := Executor{
		WaitGroup:  new(sync.WaitGroup),
		Collection: getMongoCollection("gigatiles", "optimus_content", mongoDbHost),
	}
	grpcServer := grpc.NewServer()
	pb.RegisterCDEServiceServer(grpcServer, &s)
	return grpcServer.Serve(lis)
}

func startRESTServer(address, grpcAddress string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(runtime.DefaultHeaderMatcher), runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: false, EnumsAsInts: true, EmitDefaults: true}))
	opts := []grpc.DialOption{grpc.WithInsecure()} // Register ping
	err := pb.RegisterCDEServiceHandlerFromEndpoint(ctx, mux, grpcAddress, opts)
	failOnError(err, "Error while starting rest server")
	return http.ListenAndServe(address, mux)
}


func init(){
	initializeProcess()
}

func main() {
	// fire the gRPC server in a goroutine
	go func() {
		err = startGRPCServer(grpcPort)
		failOnError(err, "Erro while starting grpc server")
	}()
	// fire the REST server in a goroutine
	go func() {
		err := startRESTServer(restPort, grpcPort)
		failOnError(err, "Error while starting rest server")
	}()
	select {}
}

func loadEnv() {
	mongoDbHost = os.Getenv("MONGO_HOST")
	grpcPort = os.Getenv("GRPC_PORT")
	restPort = os.Getenv("REST_PORT")
	youtubeFlagKey := os.Getenv("YOUTUBE_APIKEY")
	youtubeApiKeys = strings.Split(youtubeFlagKey, ",")
}

func initializeProcess() {
	fmt.Println("Welcome to init() function")
	err := godotenv.Load()
	if err != nil {
		log.Println(err.Error())
	}
	loadEnv()
	currentIndex = 0
	currentKey = youtubeApiKeys[currentIndex]
}

