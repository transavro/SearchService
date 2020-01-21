package main

import (
	pb "github.com/transavro/SearchService/proto"
	"github.com/sahilm/fuzzy"
)


type Server struct {
	Tiles TileArray
}

func(s *Server) Search( query *pb.SearchQuery, stream pb.CDEService_SearchServer) error {
	results := fuzzy.FindFrom(query.Query, s.Tiles)
	for _, r := range results {
		err := stream.Send(&s.Tiles[r.Index])
		if err != nil {
			return  err
		}
	}
	return nil
}
