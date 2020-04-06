package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/joho/godotenv"
	pbSch "github.com/transavro/ScheduleService/proto"
	pb "github.com/transavro/SearchService/proto"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

type nullawareStrDecoder struct{}

type YTSearch struct {
	Kind          string `json:"kind"`
	NextPageToken string `json:"nextPageToken"`
	Items         []struct {
		Kind string `json:"kind"`
		ID   struct {
			Kind    string `json:"kind"`
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Thumbnails  struct {
				Default struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"default"`
				Medium struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"medium"`
				High struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"high"`
			} `json:"thumbnails"`
		} `json:"snippet"`
	} `json:"items"`
}

var (
	mongoDbHost, grpcPort, restPort string
	currentIndex                               = 0
	youtubeApiKeys                             []string
	currentKey string
)

func (nullawareStrDecoder) DecodeValue(_ bsoncodec.DecodeContext, vr bsonrw.ValueReader, val reflect.Value) error {
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

//func unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
//	log.Println("unaryInterceptor")
//	err := checkingJWTToken(ctx)
//	if err != nil {
//		return nil, err
//	}
//	return handler(ctx, req)
//}

//func checkingJWTToken(ctx context.Context) error {
//
//	meta, ok := metadata.FromIncomingContext(ctx)
//	if !ok {
//		return status.Error(codes.NotFound, fmt.Sprintf("no auth meta-data found in request"))
//	}
//
//	token := meta["token"]
//
//	if len(token) == 0 {
//		return status.Error(codes.NotFound, fmt.Sprintf("Token not found"))
//	}
//
//	// calling auth service
//	conn, err := grpc.Dial(":7757", grpc.WithInsecure())
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer conn.Close()
//
//	// Auth here
//	authClient := pbAuth.NewAuthServiceClient(conn)
//	_, err = authClient.ValidateToken(context.Background(), &pbAuth.Token{
//		Token: token[0],
//	})
//	if err != nil {
//		return status.Error(codes.NotFound, fmt.Sprintf("Invalid token:  %s ", err))
//	} else {
//		return nil
//	}
//}

// streamAuthIntercept intercepts to validate authorization
//func streamIntercept(server interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, ) error {
//	err := checkingJWTToken(stream.Context())
//	if err != nil {
//		return err
//	}
//	return handler(server, stream)
//}

func startGRPCServer(address string) error {
	// create a listener on TCP port
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	s := Server{
		WaitGroup:  new(sync.WaitGroup),
		Collection: getMongoCollection("transavro", "optimus_content", mongoDbHost),
	}
	// TODO revert this changes
	//serverOptions := []grpc.ServerOption{grpc.UnaryInterceptor(unaryInterceptor), grpc.StreamInterceptor(streamIntercept)}
	serverOptions := []grpc.ServerOption{}
	// attach the Ping service to the server
	grpcServer := grpc.NewServer(serverOptions...)

	// attach the Ping service to the server
	pb.RegisterCDEServiceServer(grpcServer, &s) // start the server
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
	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(runtime.DefaultHeaderMatcher), runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: false, EnumsAsInts: true, EmitDefaults: true}))
	opts := []grpc.DialOption{grpc.WithInsecure()} // Register ping
	err := pb.RegisterCDEServiceHandlerFromEndpoint(ctx, mux, grpcAddress, opts)
	if err != nil {
		return fmt.Errorf("could not register service Ping: %s", err)
	}
	log.Printf("starting HTTP/1.1 REST server on %s", address)
	return http.ListenAndServe(address, mux)
}

func getMongoCollection(dbName, collectionName, mongoHost string) *mongo.Collection {
	// Register custom codecs for protobuf Timestamp and wrapper types
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoHost), options.Client().SetRegistry(bson.NewRegistryBuilder().
		RegisterDecoder(reflect.TypeOf(""), nullawareStrDecoder{}).
		Build(), ))

	if err != nil {
		log.Println("Error while making collection obj ")
		log.Fatal(err)
	}
	return mongoClient.Database(dbName).Collection(collectionName)
}

func main() {
	initializeProcess()

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

func makeSugPL(query string) mongo.Pipeline {
	// creating pipes for mongo aggregation for recommedation
	stages := mongo.Pipeline{}
	stages = append(stages, bson.D{{"$match", bson.M{"$text": bson.M{"$search": query}}}})
	stages = append(stages, bson.D{{"$match", bson.M{"content.publishstate": bson.M{"$ne": false}}}})
	stages = append(stages, bson.D{{"$sort", bson.M{"score": bson.M{"$meta": "textScore"}}}})
	stages = append(stages, bson.D{{"$lookup", bson.M{"from": "optimus_monetize", "localField": "refid", "foreignField": "refid", "as": "play"}}})
	stages = append(stages, bson.D{{"$replaceRoot", bson.M{"newRoot": bson.M{"$mergeObjects": bson.A{bson.M{"$arrayElemAt": bson.A{"$play", 0}}, "$$ROOT"}}}}}) //adding stage 3  ==> https://docs.mongodb.com/manual/reference/operator/aggregation/mergeObjects/#exp._S_mergeObjects
	stages = append(stages, bson.D{{"$project", bson.M{"play": 0}}})
	stages = append(stages, bson.D{{"$project", bson.M{
		"_id":          0,
		"title":        "$metadata.title",
		"poster":       "$media.landscape",
		"portriat":     "$media.portrait",
		"video":        "$media.video",
		"type":         "$tiletype",
		"isDetailPage": "$content.detailpage",
		"contentId":    "$refid",
		"play":         "$contentavailable",
	}}})
	return stages
}

type Server struct {
	*sync.WaitGroup
	*mongo.Collection
}

func (s *Server) Search(ctx context.Context, query *pb.SearchQuery) (*pb.SearchResponse, error) {
	start := time.Now()
	s.Add(2)
	searchResult := new([]*pbSch.Content)
	go s.YoutubeSearch(query.GetQuery(), searchResult)
	go s.DBSearch(ctx, query.GetQuery(), searchResult)
	s.Wait()
	log.Println("Served Result in ", time.Since(start))
	return &pb.SearchResponse{ContentTile: *searchResult}, nil
}

func (s Server) DBSearch(ctx context.Context, query string, searchResult *[]*pbSch.Content) {
	start := time.Now()
	cur, err := s.Aggregate(ctx, makeSugPL(query), options.Aggregate().SetAllowDiskUse(false))
	if err != nil {
		panic(err)
	}
	err = cur.All(ctx, searchResult)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("mongo result ==> ", time.Since(start))
	s.Done()
}

func (s Server) DBStreamSearch(ctx context.Context, query string, stream pb.CDEService_SearchStreamServer) {
	start := time.Now()
	cur, err := s.Aggregate(ctx, makeSugPL(query), options.Aggregate().SetAllowDiskUse(false))
	if err != nil {
		panic(err)
	}
	var content pbSch.Content
	for cur.Next(ctx) {
		if err = cur.Decode(&content); err != nil {
			log.Fatal(err)
		}
		if err = stream.Send(&content); err != nil {
			log.Fatal(err)
		}
	}
	log.Println("mongo result ==> ", time.Since(start))
	s.Done()
}

func (s Server) YoutubeSearch(query string, primeResult *[]*pbSch.Content) error {
	start := time.Now()
	req, err := http.NewRequest("GET", "https://www.googleapis.com/youtube/v3/search", nil)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Add("key", currentKey)
	q.Add("maxResults", "50")
	q.Add("q", query)
	q.Add("part", "snippet")

	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == 200 {
		var searchResp YTSearch
		err = json.NewDecoder(resp.Body).Decode(&searchResp)
		if err != nil {
			log.Println("got error 1ch  ", err.Error())
			return err
		}

		for index, item := range searchResp.Items {
			var contentTile pbSch.Content
			var play pbSch.Play
			if index >= 20 {
				break
			}
			contentTile.Title = item.Snippet.Title
			contentTile.Poster = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.Portriat = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.IsDetailPage = false
			contentTile.Type = pbSch.TileType_ImageTile

			play.Package = "com.google.android.youtube"
			if item.ID.Kind == "youtube#video" {
				play.Target = item.ID.VideoID
				play.Source = "Youtube"
				play.Type = "CWYT_VIDEO"
				contentTile.Play = []*pbSch.Play{&play}
				*primeResult = append(*primeResult, &contentTile)
			}
		}
		s.Done()
		log.Println("From youtube ==> ", time.Since(start))
		return resp.Body.Close()
	} else {
		log.Println("old Api Key ", currentKey)
		currentIndex = currentIndex + 1
		if len(youtubeApiKeys) > currentIndex {
			currentKey = youtubeApiKeys[currentIndex]
			log.Println("new Api Kye ", currentKey)
			return s.YoutubeSearch(query, primeResult)
		} else {
			panic(errors.New("Youtube api keys got over."))
		}
	}
}

func (s Server) YoutubeStreamSearch(query string, stream pb.CDEService_SearchStreamServer) error {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/youtube/v3/search", nil)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Add("key", currentKey)
	q.Add("maxResults", "50")
	q.Add("q", query)
	q.Add("part", "snippet")

	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == 200 {
		var searchResp YTSearch
		err = json.NewDecoder(resp.Body).Decode(&searchResp)
		if err != nil {
			log.Println("got error 1ch  ", err.Error())
			return err
		}

		for index, item := range searchResp.Items {
			var contentTile pbSch.Content
			var play pbSch.Play
			if index >= 20 {
				break
			}
			contentTile.Title = item.Snippet.Title
			contentTile.Poster = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.Portriat = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.IsDetailPage = false
			contentTile.Type = pbSch.TileType_ImageTile

			play.Package = "com.google.android.youtube"
			if item.ID.Kind == "youtube#video" {
				play.Target = item.ID.VideoID
				play.Source = "Youtube"
				play.Type = "CWYT_VIDEO"
				contentTile.Play = []*pbSch.Play{&play}
				if err = stream.Send(&contentTile); err != nil {
					return err
				}
			}
		}

		s.Done()
		return resp.Body.Close()
	} else {
		currentIndex = currentIndex + 1
		if len(youtubeApiKeys) > currentIndex {
			currentKey = youtubeApiKeys[currentIndex]
			return s.YoutubeStreamSearch(query, stream)
		} else {
			panic(errors.New("Youtube api keys got over."))
		}
	}
}

func (s *Server) SearchStream(query *pb.SearchQuery, stream pb.CDEService_SearchStreamServer) error {
	s.Add(2)
	go s.YoutubeStreamSearch(query.GetQuery(), stream)
	go s.DBStreamSearch(stream.Context(), query.GetQuery(), stream)
	s.Wait()
	return nil
}
