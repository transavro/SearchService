package main

import (
	"context"
	"encoding/json"
	"errors"
	pb "github.com/transavro/SearchService/gen"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"net/http"
	"sync"
	"time"
)


type Executor struct{
	*mongo.Collection
	*sync.WaitGroup
}

func (s *Executor) SearchText(ctx context.Context, query *pb.SearchQuery) (*pb.SearchTextResponse, error) {
	if len(query.GetQuery()) == 0 {
		return nil, errors.New("Query cannot be empty")
	}
	cur, err := s.Aggregate(ctx, makeTextPL(query.GetQuery()))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []string{}
	tmp := new(Temp)
	for cur.Next(ctx){
		err = cur.Decode(tmp)
		if err != nil {
			return nil, err
		} 
		result = append(result, tmp.Title)
	}
	return &pb.SearchTextResponse{Result: result}, nil
}


func (s *Executor) Search(ctx context.Context, query *pb.SearchQuery) (*pb.SearchResponse, error) {
	if len(query.GetQuery()) == 0 {
		return nil, errors.New("Query cannot be empty")
	}
	start := time.Now()
	s.Add(1)
	searchResult := new([]*pb.Content)
	//go s.YoutubeSearch(query.GetQuery(), searchResult)
	go s.DBSearch(ctx, query.GetQuery(), searchResult)
	s.Wait()
	log.Println("Served Result in ", time.Since(start))
	return &pb.SearchResponse{ContentTile: *searchResult}, nil
}

func (s *Executor) SearchStream(query *pb.SearchQuery, stream pb.CDEService_SearchStreamServer) error {
	if len(query.GetQuery()) == 0 {
		return errors.New("Query cannot be empty")
	}
	s.Add(1)
	//go s.YoutubeStreamSearch(query.GetQuery(), stream)
	go s.DBStreamSearch(stream.Context(), query.GetQuery(), stream)
	s.Wait()
	return nil
}


// youtube Search with stream result
func (s *Executor) YoutubeStreamSearch(query string, stream pb.CDEService_SearchStreamServer) error {
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
			var contentTile pb.Content
			var play pb.Play
			if index >= 20 {
				break
			}
			contentTile.Title = item.Snippet.Title
			contentTile.Poster = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.Portriat = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.IsDetailPage = false
			contentTile.Type = pb.TileType_ImageTile

			play.Package = "com.google.android.youtube"
			if item.ID.Kind == "youtube#video" {
				play.Target = item.ID.VideoID
				play.Source = "Youtube"
				play.Type = "CWYT_VIDEO"
				contentTile.Play = []*pb.Play{&play}
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

// youtube search with result
func (s *Executor) YoutubeSearch(query string, primeResult *[]*pb.Content) error {
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
			var contentTile pb.Content
			var play pb.Play
			if index >= 20 {
				break
			}
			contentTile.Title = item.Snippet.Title
			contentTile.Poster = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.Portriat = []string{item.Snippet.Thumbnails.Medium.URL}
			contentTile.IsDetailPage = false
			contentTile.Type = pb.TileType_ImageTile

			play.Package = "com.google.android.youtube"
			if item.ID.Kind == "youtube#video" {
				play.Target = item.ID.VideoID
				play.Source = "Youtube"
				play.Type = "CWYT_VIDEO"
				contentTile.Play = []*pb.Play{&play}
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

// db search
func (s *Executor) DBSearch(ctx context.Context, query string, searchResult *[]*pb.Content) {
	start := time.Now()
	cur, err := s.Aggregate(ctx, makeSugPL(query), options.Aggregate().SetAllowDiskUse(false))
	if err != nil {
		panic(err)
	}
	err = cur.All(ctx, searchResult)
	if err != nil {
		log.Fatal(err)
	}
	for i, content := range *searchResult {
		println(i,"                  " ,content.String())
	}
	log.Println("mongo result ==> ", time.Since(start))
	s.Done()
}


// db search with stream result
func (s *Executor) DBStreamSearch(ctx context.Context, query string, stream pb.CDEService_SearchStreamServer) {
	start := time.Now()
	cur, err := s.Aggregate(ctx, makeTempPL(query), options.Aggregate().SetAllowDiskUse(false))
	if err != nil {
		panic(err)
	}
	var content pb.Content
	var play pb.Play
	var temp Temp
	for cur.Next(ctx) {
		if err = cur.Decode(&temp); err != nil {
			continue
		}
		content.Title = temp.Title
		content.ContentId = temp.ContentID
		if temp.Poster != nil && len(temp.Poster) > 0 {
			content.Poster = temp.Poster
		}
		if temp.Portriat != nil && len(temp.Portriat) > 0 {
			content.Portriat = temp.Portriat
		}
		content.IsDetailPage = temp.IsDetailPage
		content.Type = pb.TileType_ImageTile

		if len(temp.Contenttype) > 0 {
			play.Type = temp.Contenttype
		}

		if len(temp.Package) > 0 {
			play.Package = temp.Package
		}

		if len(temp.Source) > 0 {
			play.Source = temp.Source
		}

		if len(temp.Target) > 0 {
			play.Target = temp.Target[0]
		}

		content.Play = append(content.Play, &play)

		if err = stream.Send(&content); err != nil {
			log.Fatal(err)
		}
	}
	log.Println("mongo result ==> ", time.Since(start))
	s.Done()
}

