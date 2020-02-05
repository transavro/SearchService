package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/joho/godotenv"
	"github.com/sahilm/fuzzy"
	pbAuth "github.com/transavro/AuthService/proto"
	pbSch "github.com/transavro/ScheduleService/proto"
	pb "github.com/transavro/SearchService/proto"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"time"
)

type nullawareStrDecoder struct{}

var mongoDbHost, redisPort, grpcPort, restPort  string

func (nullawareStrDecoder) DecodeValue(dctx bsoncodec.DecodeContext, vr bsonrw.ValueReader, val reflect.Value) error {
	if !val.CanSet() || val.Kind() != reflect.String {
		return errors.New("bad type or not settable")
	}
	var str string
	var err error
	switch vr.Type() {
	case bsontype.String:
		if str, err = vr.ReadString(); err != nil {
			return err
		}
	case bsontype.Null: // THIS IS THE MISSING PIECE TO HANDLE NULL!
		if err = vr.ReadNull(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("cannot decode %v into a string type", vr.Type())
	}

	val.SetString(str)
	return nil
}



var targetArray TileArray

type TileArray []pbSch.Content

func (e TileArray) String(i int) string  {
	return e[i].Title
}

func(e TileArray) Len() int {
	return len(e)
}

func unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Println("unaryInterceptor")
	err := checkingJWTToken(ctx)
	if err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func checkingJWTToken(ctx context.Context) error{

	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.NotFound, fmt.Sprintf("no auth meta-data found in request" ))
	}

	token := meta["token"]

	if len(token) == 0 {
		return  status.Error(codes.NotFound, fmt.Sprintf("Token not found" ))
	}

	// calling auth service
	conn, err := grpc.Dial(":7757", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Auth here
	authClient := pbAuth.NewAuthServiceClient(conn)
	_, err = authClient.ValidateToken(context.Background(), &pbAuth.Token{
		Token: token[0],
	})
	if err != nil {
		return  status.Error(codes.NotFound, fmt.Sprintf("Invalid token:  %s ", err ))
	}else {
		return nil
	}
}

// streamAuthIntercept intercepts to validate authorization
func streamIntercept(server interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, ) error {
	err := checkingJWTToken(stream.Context())
	if err != nil {
		return err
	}
	return handler(server, stream)
}

func startGRPCServer(address string) error {
	// create a listener on TCP port
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}  // create a server instance
	if err != nil {
		return err
	}

	s := Server {
		Tiles:         targetArray ,
	}

	// TODO revert this changes
	serverOptions := []grpc.ServerOption{grpc.UnaryInterceptor(unaryInterceptor), grpc.StreamInterceptor(streamIntercept)}
	// attach the Ping service to the server
	grpcServer := grpc.NewServer(serverOptions...)

	// attach the Ping service to the server
	pb.RegisterCDEServiceServer(grpcServer, &s)  // start the server
	//log.Printf("starting HTTP/2 gRPC server on %s", address)
	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %s", err)
	}
	return nil
}

func startRESTServer(address, grpcAddress string) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(runtime.DefaultHeaderMatcher), runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName:false, EnumsAsInts:true, EmitDefaults:true}))

	opts := []grpc.DialOption{grpc.WithInsecure()}  // Register ping
	err := pb.RegisterCDEServiceHandlerFromEndpoint(ctx, mux, grpcAddress, opts)
	if err != nil {
		return fmt.Errorf("could not register service Ping: %s", err)
	}

	log.Printf("starting HTTP/1.1 REST server on %s", address)
	http.ListenAndServe(address, mux)
	return nil
}

func getMongoCollection(dbName, collectionName, mongoHost string )  *mongo.Collection {
	// Register custom codecs for protobuf Timestamp and wrapper types
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	mongoClient, err :=  mongo.Connect(ctx, options.Client().ApplyURI(mongoHost), options.Client().SetRegistry(bson.NewRegistryBuilder().
		RegisterDecoder(reflect.TypeOf(""), nullawareStrDecoder{}).
		Build(),))

	if err != nil {
		log.Println("Error while making collection obj ")
		log.Fatal(err)
	}
	return mongoClient.Database(dbName).Collection(collectionName)
}

func main()  {
	initializeProcess();

	// fire the gRPC server in a goroutine
	go func() {
		err := startGRPCServer(grpcPort)
		if err != nil {
			log.Fatalf("failed to start gRPC server: %s", err)
		}
	}()

	// fire the REST server in a goroutine
	go func() {
		err := startRESTServer(restPort, grpcPort)
		if err != nil {
			log.Fatalf("failed to start gRPC server: %s", err)
		}
	}()

	// infinite loop
	select {}
}

func loadEnv(){
	mongoDbHost = os.Getenv("MONGO_HOST")
	redisPort = os.Getenv("REDIS_PORT")
	grpcPort = os.Getenv("GRPC_PORT")
	restPort = os.Getenv("REST_PORT")
}

func initializeProcess()  {
	fmt.Println("Welcome to init() function")
	err := godotenv.Load()
	if err != nil {
		log.Println(err.Error())
	}
	loadEnv()
	primeTiles := getMongoCollection("optimus", "contents", mongoDbHost)
	loadingInToArray(primeTiles)
}

func loadingInToArray(tileCollection *mongo.Collection){
	// creating pipes for mongo aggregation
	pipeline := mongo.Pipeline{}

	//stage1 communicating with diff collection
	stage1 := bson.D{{"$lookup", bson.M{"from": "monetizes", "localField": "ref_id", "foreignField": "ref_id", "as": "play"}}}

	pipeline = append(pipeline, stage1)

	//stage2 unwinding the schema accroding in the lookup result
	stage2 := bson.D{{"$unwind", "$play"}}
	pipeline = append(pipeline, stage2)


	//stage4 groupby stage
	stage4 := bson.D{{"$group", bson.D{{"_id", bson.D{
		{"created_at", "$created_at"},
		{"updated_at", "$updated_at"},
		{"contentId", "$ref_id"},
		{"releaseDate", "$metadata.releaseDate"},
		{"year", "$metadata.year"},
		{"imdbId", "$metadata.imdbId"},
		{"rating", "$metadata.rating"},
		{"viewCount", "$metadata.viewCount"},

	}}, {"contentTile", bson.D{{"$push", bson.D{
		{"title", "$metadata.title"},
		{"portrait", "$media.portrait",},
		{"poster", "$media.landscape"},
		{"video", "$media.video"},
		{"contentId", "$ref_id"},
		{"isDetailPage", "$content.detailPage"},
		{"type", "$tileType"},
		{"play", "$play.contentAvailable"},
	}}}}}}}

	pipeline = append(pipeline, stage4)

	//stage6 unwinding the resultant array
	stage6 := bson.D{{"$unwind", "$contentTile"}}
	pipeline = append(pipeline, stage6)


	//stage6 moulding according to the deliver schema
	stage7 := bson.D{{"$project", bson.D{
		{"_id", 0},
		{"title", "$contentTile.title"},
		{"poster", "$contentTile.poster"},
		{"portriat", "$contentTile.portrait"},
		{"type", "$contentTile.type"},
		{"isDetailPage", "$contentTile.isDetailPage"},
		{"contentId", "$contentTile.contentId"},
		{"play", "$contentTile.play"},
		{"video", "$contentTile.video"},
	}}}
	pipeline = append(pipeline, stage7)

	cur, err := tileCollection.Aggregate(context.Background(), pipeline)
	if err != nil {
		log.Println("Error while find ")
		log.Fatal(err)
	}

	for cur.Next(context.Background()) {
		var content pbSch.Content
		err = cur.Decode(&content)
		if err != nil {
			log.Fatal(err)
		}
		targetArray = append(targetArray, content)
	}
}

type Server struct {
	Tiles TileArray
}

func(s *Server) Search( ctx context.Context, query *pb.SearchQuery) (*pb.SearchResponse, error) {
	log.Println("search********")
	results := fuzzy.FindFrom(query.Query, s.Tiles)
	var searchResult []*pbSch.Content
	for _, r := range results {
		searchResult = append(searchResult , &s.Tiles[r.Index])
	}
	return &pb.SearchResponse{ContentTile: searchResult}, nil
}


func(s *Server) SearchStream( query *pb.SearchQuery, stream pb.CDEService_SearchStreamServer) error {
	log.Println("SearchStream********")
	results := fuzzy.FindFrom(query.Query, s.Tiles)
	for _, r := range results {
		err := stream.Send(&s.Tiles[r.Index])
		if err != nil {
			return  err
		}
	}
	return nil
}