package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	cloudwalker "github.com/transavro/SearchService/gen"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Executor struct {
	movieCollection *mongo.Collection
	showCollection  *mongo.Collection
	*sync.WaitGroup
}

func (s *Executor) SearchText(ctx context.Context, query *cloudwalker.SearchQuery) (*cloudwalker.SearchTextResponse, error) {

	println("HIT TEXT ZALA")
	if len(query.GetQuery()) == 0 {
		return nil, errors.New("Query cannot be empty")
	}
	cur, err := s.movieCollection.Aggregate(ctx, makeTextPL(query.GetQuery()))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []string{}
	tmp := new(Temp)
	for cur.Next(ctx) {
		err = cur.Decode(tmp)
		if err != nil {
			return nil, err
		}
		result = append(result, tmp.Title)
	}
	return &cloudwalker.SearchTextResponse{Result: result}, nil
}

func (s *Executor) Search(ctx context.Context, query *cloudwalker.SearchQuery) (*cloudwalker.SearchResponse, error) {
	println("HIT ZALA ", query.Query)
	// if len(query.GetQuery()) == 0 {
	// 	return nil, errors.New("Query cannot be empty")
	// }

	showFilterKeys := []string{"seasonnumber", "episodenumber", "source", "showtitle"}
	start := time.Now()

	searchResult := new([]*cloudwalker.ContentDelivery)
	isShowFilter := false

	// check if it is a show filter
	for k, _ := range query.SearchMeta {
		if contains(showFilterKeys, k) {
			isShowFilter = true
		}
	}
	//if its a show filter remove or convert all the show-non filter keys
	if isShowFilter {
		showfilterMap := make(map[string]*cloudwalker.FilterKey)
		for k, v := range query.SearchMeta {
			//if a key is not in a valid-keys-array change it & add.
			if !contains(showFilterKeys, k) {
				// if its source
				if k == "content.sources" {
					showfilterMap["source"] = v
				} else if k == "metadata.title" {
					showfilterMap["showtitle"] = v
				}
			} else {
				showfilterMap[k] = v
			}
		}
		query.SearchMeta = showfilterMap
		s.Add(1)
		go s.ShowJwSearch(query, searchResult)
	} else {
		s.Add(2)
		go s.ShowJwSearch(query, searchResult)
		go s.MovieJwSearch(query, searchResult)
	}

	s.Wait()
	log.Println("Served Result in ", time.Since(start))
	return &cloudwalker.SearchResponse{ContentTiles: *searchResult}, nil
}

func (s *Executor) SearchStream(query *cloudwalker.SearchQuery, stream cloudwalker.CDEService_SearchStreamServer) error {
	if len(query.GetQuery()) == 0 {
		return errors.New("Query cannot be empty")
	}
	s.Add(1)
	//go s.YoutubeStreamSearch(query.GetQuery(), stream)
	go s.DBStreamSearch(stream.Context(), query.GetQuery(), stream)
	s.Wait()
	return nil
}

// db search
func (s *Executor) DBSearch(ctx context.Context, query *cloudwalker.SearchQuery, searchResult *[]*cloudwalker.Optimus) {
	start := time.Now()
	cur, err := s.movieCollection.Aggregate(ctx, makeOptiSugPL(query), options.Aggregate().SetAllowDiskUse(false))
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

// db search with stream result
func (s *Executor) DBStreamSearch(ctx context.Context, query string, stream cloudwalker.CDEService_SearchStreamServer) {
	start := time.Now()
	cur, err := s.movieCollection.Aggregate(ctx, makeTempPL(query), options.Aggregate().SetAllowDiskUse(false))
	if err != nil {
		panic(err)
	}
	var content *cloudwalker.Optimus

	for cur.Next(ctx) {
		if err = cur.Decode(content); err != nil {
			continue
		}
		if err = stream.Send(content); err != nil {
			log.Fatal(err)
		}
	}
	log.Println("mongo result ==> ", time.Since(start))
	s.Done()
}

// db search with stream result
func (s *Executor) CdeClick(ctx context.Context, tileId *cloudwalker.TileId) (*cloudwalker.Resp, error) {
	println("TILE CLIKED ", tileId.String())
	filter := bson.D{
		{
			Key:   "refid",
			Value: tileId.GetId(),
		},
	}

	update := bson.D{
		{
			Key:   "$inc",
			Value: bson.M{"metadata.viewcount": 1.0},
		},
	}
	_, err := s.movieCollection.UpdateOne(ctx, filter, update)
	return &cloudwalker.Resp{Result: true}, err
}

func (s *Executor) ShowJwSearch(query *cloudwalker.SearchQuery, optimus *[]*cloudwalker.ContentDelivery) {
	println("Show Search ")
	cur, err := s.showCollection.Aggregate(context.Background(), showSearchPL(query))
	if err != nil {
		log.Fatal(err)
	}
	println("Show Search 1")
	defer cur.Decode(context.Background())
	for cur.Next(context.Background()) {
		delivery := new(cloudwalker.ContentDelivery)
		err = cur.Decode(delivery)
		if err != nil {
			log.Fatal(err)
		}
		println("Show Search looping..", delivery.Title)
		*optimus = append(*optimus, delivery)
	}
	s.Done()
}

func (s *Executor) ShowJwTEXTSearch(query *cloudwalker.SearchQuery, optimus *[]string) {
	println("Show Text Search ")
	cur, err := s.showCollection.Aggregate(context.Background(), showSearchTextPL(query))
	if err != nil {
		log.Fatal(err)
	}
	println("Show Text Search 1")
	defer cur.Decode(context.Background())
	for cur.Next(context.Background()) {
		*optimus = append(*optimus, cur.Current.Lookup("title").StringValue())
	}
	s.Done()
}

func (s *Executor) MovieJwTEXTSearch(query *cloudwalker.SearchQuery, optimus *[]string) {
	println("Movie Text Search ")
	cur, err := s.movieCollection.Aggregate(context.Background(), movieSearchTextPL(query))
	if err != nil {
		log.Fatal(err)
	}
	println("Movie Text Search 1")
	defer cur.Decode(context.Background())
	for cur.Next(context.Background()) {
		*optimus = append(*optimus, cur.Current.Lookup("title").StringValue())
	}
	s.Done()
}

func (s *Executor) MovieJwSearch(query *cloudwalker.SearchQuery, optimus *[]*cloudwalker.ContentDelivery) {
	println("Movie Search ")
	cur, err := s.movieCollection.Aggregate(context.Background(), movieSearchPL(query))
	if err != nil {
		log.Fatal(err)
	}
	println("Movie Search 1")
	defer cur.Decode(context.Background())
	for cur.Next(context.Background()) {
		delivery := new(cloudwalker.ContentDelivery)

		err = cur.Decode(delivery)
		if err != nil {
			log.Fatal(err)
		}
		println("Movie Search looping..", delivery.Title)
		*optimus = append(*optimus, delivery)
	}
	s.Done()
}

// // youtube Search with stream result
// func (s *Executor) YoutubeStreamSearch(query string, stream cloudwalker.CDEService_SearchStreamServer) error {
// 	req, err := http.NewRequest("GET", "https://www.googleapis.com/youtube/v3/search", nil)
// 	if err != nil {
// 		return err
// 	}
// 	q := req.URL.Query()
// 	q.Add("key", currentKey)
// 	q.Add("maxResults", "50")
// 	q.Add("q", query)
// 	q.Add("part", "snippet")

// 	req.URL.RawQuery = q.Encode()
// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return err
// 	}
// 	if resp.StatusCode == 200 {
// 		var searchResp YTSearch
// 		err = json.NewDecoder(resp.Body).Decode(&searchResp)
// 		if err != nil {
// 			log.Println("got error 1ch  ", err.Error())
// 			return err
// 		}

// 		for index, item := range searchResp.Items {
// 			var contentTile cloudwalker.Optimus
// 			if index >= 20 {
// 				break
// 			}
// 			contentTile.Title = item.Snippet.Title
// 			contentTile.Poster = []string{item.Snippet.Thumbnails.Medium.URL}
// 			contentTile.Portriat = []string{item.Snippet.Thumbnails.Medium.URL}
// 			contentTile.IsDetailPage = false
// 			contentTile.Type = cloudwalker.TileType_ImageTile

// 			play.Package = "com.google.android.youtube"
// 			if item.ID.Kind == "youtube#video" {
// 				play.Target = item.ID.VideoID
// 				play.Source = "Youtube"
// 				play.Type = "CWYT_VIDEO"
// 				contentTile.Play = []*cloudwalker.Play{&play}
// 				if err = stream.Send(&contentTile); err != nil {
// 					return err
// 				}
// 			}
// 		}

// 		s.Done()
// 		return resp.Body.Close()
// 	} else {
// 		currentIndex = currentIndex + 1
// 		if len(youtubeApiKeys) > currentIndex {
// 			currentKey = youtubeApiKeys[currentIndex]
// 			return s.YoutubeStreamSearch(query, stream)
// 		} else {
// 			panic(errors.New("Youtube api keys got over."))
// 		}
// 	}
// }

// // youtube search with result
// func (s *Executor) YoutubeSearch(query string, primeResult *[]*cloudwalker.Content) error {
// 	start := time.Now()
// 	req, err := http.NewRequest("GET", "https://www.googleapis.com/youtube/v3/search", nil)
// 	if err != nil {
// 		return err
// 	}
// 	q := req.URL.Query()
// 	q.Add("key", currentKey)
// 	q.Add("maxResults", "50")
// 	q.Add("q", query)
// 	q.Add("part", "snippet")

// 	req.URL.RawQuery = q.Encode()
// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return err
// 	}
// 	if resp.StatusCode == 200 {
// 		var searchResp YTSearch
// 		err = json.NewDecoder(resp.Body).Decode(&searchResp)
// 		if err != nil {
// 			log.Println("got error 1ch  ", err.Error())
// 			return err
// 		}

// 		for index, item := range searchResp.Items {
// 			var contentTile cloudwalker.Content
// 			var play cloudwalker.Play
// 			if index >= 20 {
// 				break
// 			}
// 			contentTile.Title = item.Snippet.Title
// 			contentTile.Poster = []string{item.Snippet.Thumbnails.Medium.URL}
// 			contentTile.Portriat = []string{item.Snippet.Thumbnails.Medium.URL}
// 			contentTile.IsDetailPage = false
// 			contentTile.Type = cloudwalker.TileType_ImageTile

// 			play.Package = "com.google.android.youtube"
// 			if item.ID.Kind == "youtube#video" {
// 				play.Target = item.ID.VideoID
// 				play.Source = "Youtube"
// 				play.Type = "CWYT_VIDEO"
// 				contentTile.Play = []*cloudwalker.Play{&play}
// 				*primeResult = append(*primeResult, &contentTile)
// 			}
// 		}
// 		s.Done()
// 		log.Println("From youtube ==> ", time.Since(start))
// 		return resp.Body.Close()
// 	} else {
// 		log.Println("old Api Key ", currentKey)
// 		currentIndex = currentIndex + 1
// 		if len(youtubeApiKeys) > currentIndex {
// 			currentKey = youtubeApiKeys[currentIndex]
// 			log.Println("new Api Kye ", currentKey)
// 			return s.YoutubeSearch(query, primeResult)
// 		} else {
// 			panic(errors.New("Youtube api keys got over."))
// 		}
// 	}
// }
