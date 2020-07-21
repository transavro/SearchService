package main
//
//import (
//"context"
//"errors"
//"fmt"
//"log"
//"net"
//"net/http"
//"os"
//"reflect"
//"strings"
//"sync"
//"time"
//
//"github.com/grpc-ecosystem/grpc-gateway/runtime"
//"github.com/joho/godotenv"
//pb "github.com/transavro/SearchService/gen"
//"go.mongodb.org/mongo-driver/bson"
//"go.mongodb.org/mongo-driver/bson/bsoncodec"
//"go.mongodb.org/mongo-driver/bson/bsonrw"
//"go.mongodb.org/mongo-driver/bson/bsontype"
//"go.mongodb.org/mongo-driver/mongo"
//"go.mongodb.org/mongo-driver/mongo/options"
//"google.golang.org/grpc"
//)
//
//type nullawareStrDecoder struct{}
//
//
//var (
//	mongoDbHost, grpcPort, restPort string
//	currentIndex                    = 0
//	youtubeApiKeys                  []string
//	currentKey                      string
//)
//
//func (nullawareStrDecoder) DecodeValue(_ bsoncodec.DecodeContext, vr bsonrw.ValueReader, val reflect.Value) error {
//	if !val.CanSet() || val.Kind() != reflect.String {
//		return errors.New("bad type or not settable")
//	}
//	var str string
//	var err error
//	switch vr.Type() {
//	case bsontype.String:
//		if str, err = vr.ReadString(); err != nil {
//			return err
//		}
//	case bsontype.Null: // THIS IS THE MISSING PIECE TO HANDLE NULL!
//		if err = vr.ReadNull(); err != nil {
//			return err
//		}
//	default:
//		return fmt.Errorf("cannot decode %v into a string type", vr.Type())
//	}
//
//	val.SetString(str)
//	return nil
//}
//
////func unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
////	log.Println("unaryInterceptor")
////	err := checkingJWTToken(ctx)
////	if err != nil {
////		return nil, err
////	}
////	return handler(ctx, req)
////}
//
////func checkingJWTToken(ctx context.Context) error {
////
////	meta, ok := metadata.FromIncomingContext(ctx)
////	if !ok {
////		return status.Error(codes.NotFound, fmt.Sprintf("no auth meta-data found in request"))
////	}
////
////	token := meta["token"]
////
////	if len(token) == 0 {
////		return status.Error(codes.NotFound, fmt.Sprintf("Token not found"))
////	}
////
////	// calling auth service
////	conn, err := grpc.Dial(":7757", grpc.WithInsecure())
////	if err != nil {
////		log.Fatal(err)
////	}
////	defer conn.Close()
////
////	// Auth here
////	authClient := pbAuth.NewAuthServiceClient(conn)
////	_, err = authClient.ValidateToken(context.Background(), &pbAuth.Token{
////		Token: token[0],
////	})
////	if err != nil {
////		return status.Error(codes.NotFound, fmt.Sprintf("Invalid token:  %s ", err))
////	} else {
////		return nil
////	}
////}
//
//// streamAuthIntercept intercepts to validate authorization
////func streamIntercept(server interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, ) error {
////	err := checkingJWTToken(stream.Context())
////	if err != nil {
////		return err
////	}
////	return handler(server, stream)
////}
//
//func startGRPCServer(address string) error {
//	// create a listener on TCP port
//	lis, err := net.Listen("tcp", address)
//	if err != nil {
//		return fmt.Errorf("failed to listen: %v", err)
//	}
//	s := Executor{
//		WaitGroup:  new(sync.WaitGroup),
//		Collection: getMongoCollection("gigatiles", "optimus_content", mongoDbHost),
//	}
//	// TODO revert this changes
//	//serverOptions := []grpc.ServerOption{grpc.UnaryInterceptor(unaryInterceptor), grpc.StreamInterceptor(streamIntercept)}
//	serverOptions := []grpc.ServerOption{}
//	// attach the Ping service to the server
//	grpcServer := grpc.NewServer(serverOptions...)
//
//	// attach the Ping service to the server
//	pb.RegisterCDEServiceServer(grpcServer, &s) // start the server
//	//log.Printf("starting HTTP/2 gRPC server on %s", address)
//	if err := grpcServer.Serve(lis); err != nil {
//		return fmt.Errorf("failed to serve: %s", err)
//	}
//	return nil
//}
//
//func startRESTServer(address, grpcAddress string) error {
//	ctx := context.Background()
//	ctx, cancel := context.WithCancel(ctx)
//	defer cancel()
//	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(runtime.DefaultHeaderMatcher), runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: false, EnumsAsInts: true, EmitDefaults: true}))
//	opts := []grpc.DialOption{grpc.WithInsecure()} // Register ping
//	err := pb.RegisterCDEServiceHandlerFromEndpoint(ctx, mux, grpcAddress, opts)
//	if err != nil {
//		return fmt.Errorf("could not register service Ping: %s", err)
//	}
//	log.Printf("starting HTTP/1.1 REST server on %s", address)
//	return http.ListenAndServe(address, mux)
//}
//
//func getMongoCollection(dbName, collectionName, mongoHost string) *mongo.Collection {
//	// Register custom codecs for protobuf Timestamp and wrapper types
//	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
//	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoHost), options.Client().SetRegistry(bson.NewRegistryBuilder().
//		RegisterDecoder(reflect.TypeOf(""), nullawareStrDecoder{}).
//		Build()))
//
//	if err != nil {
//		log.Println("Error while making collection obj ")
//		log.Fatal(err)
//	}
//	return mongoClient.Database(dbName).Collection(collectionName)
//}
//
//func main() {
//	initializeProcess()
//
//	// fire the gRPC server in a goroutine
//	go func() {
//		err := startGRPCServer(grpcPort)
//		if err != nil {
//			log.Fatalf("failed to start gRPC server: %s", err)
//		}
//	}()
//
//	// fire the REST server in a goroutine
//	go func() {
//		err := startRESTServer(restPort, grpcPort)
//		if err != nil {
//			log.Fatalf("failed to start gRPC server: %s", err)
//		}
//	}()
//
//	// infinite loop
//	select {}
//}
//
//func loadEnv() {
//	mongoDbHost = os.Getenv("MONGO_HOST")
//	grpcPort = os.Getenv("GRPC_PORT")
//	restPort = os.Getenv("REST_PORT")
//	youtubeFlagKey := os.Getenv("YOUTUBE_APIKEY")
//	youtubeApiKeys = strings.Split(youtubeFlagKey, ",")
//}
//
//func initializeProcess() {
//	fmt.Println("Welcome to init() function")
//	err := godotenv.Load()
//	if err != nil {
//		log.Println(err.Error())
//	}
//	loadEnv()
//	currentIndex = 0
//	currentKey = youtubeApiKeys[currentIndex]
//
//}
//
//type Temp struct {
//	Title        string   `json:"title"`
//	Poster       []string `json:"poster"`
//	Portriat     []string `json:"portriat"`
//	IsDetailPage bool     `json:"isDetailPage"`
//	ContentID    string   `json:"contentId"`
//	Package      string   `json:"package"`
//	Source       string   `json:"source"`
//	Contenttype  string   `json:"contenttype"`
//	Target       []string `json:"target"`
//}
//
//func makeTempPL(query string) mongo.Pipeline {
//	stages := mongo.Pipeline{}
//	stages = append(stages, bson.D{{"$match", bson.M{"$text": bson.M{"$search": query}}}})
//	stages = append(stages, bson.D{{"$match", bson.M{"content.publishstate": bson.M{"$ne": false}}}})
//	stages = append(stages, bson.D{{"$sort", bson.M{"score": bson.M{"$meta": "textScore"}}}})
//	stages = append(stages, bson.D{{"$project", bson.M{
//		"_id":          0,
//		"title":        "$metadata.title",
//		"poster":       "$posters.landscape",
//		"portriat":     "$posters.portrait",
//		"video":        "$media.video",
//		"type":         "$tiletype",
//		"isDetailPage": "$content.detailPage",
//		"contentId":    "$ref_id",
//		"package":      "$content.package",
//		"source":       "$content.source",
//		"contenttype":  "$content.type",
//		"target":       "$content.target",
//	}}})
//	return stages
//}
//
//func makeSugPL(query string) mongo.Pipeline {
//	// creating pipes for mongo aggregation for recommedation
//	stages := mongo.Pipeline{}
//	stages = append(stages, bson.D{{"$match", bson.M{"$text": bson.M{"$search": query}}}})
//	stages = append(stages, bson.D{{"$match", bson.M{"content.publishstate": bson.M{"$ne": false}}}})
//	stages = append(stages, bson.D{{"$sort", bson.M{"score": bson.M{"$meta": "textScore"}}}})
//	stages = append(stages, bson.D{{"$lookup", bson.M{"from": "optimus_monetize", "localField": "refid", "foreignField": "refid", "as": "play"}}})
//	stages = append(stages, bson.D{{"$replaceRoot", bson.M{"newRoot": bson.M{"$mergeObjects": bson.A{bson.M{"$arrayElemAt": bson.A{"$play", 0}}, "$$ROOT"}}}}}) //adding stage 3  ==> https://docs.mongodb.com/manual/reference/operator/aggregation/mergeObjects/#exp._S_mergeObjects
//	stages = append(stages, bson.D{{"$project", bson.M{"play": 0}}})
//	stages = append(stages, bson.D{{"$project", bson.M{
//		"_id":          0,
//		"title":        "$metadata.title",
//		"poster":       "$media.landscape",
//		"portriat":     "$media.portrait",
//		"video":        "$media.video",
//		"type":         "$tiletype",
//		"isDetailPage": "$content.detailpage",
//		"contentId":    "$refid",
//		"play":         "$contentavailable",
//	}}})
//	return stages
//}