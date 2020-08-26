package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	cloudwalker "github.com/transavro/SearchService/gen"
)

type nullawareStrDecoder struct{}

func failOnError(err error, msg string) {
	if err != nil {
		log.Println(msg)
		panic(err)
	}
}

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

/*
	getting mongo collection to search from
	@param Database Name
	@param Collection Name
	@param mongoHost Url
	@return mongo Collection pointer
*/

func getMongoCollection(dbName, collectionName, mongoHost string) *mongo.Collection {
	// Register custom codecs for protobuf Timestamp and wrapper types
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoHost), options.Client().SetRegistry(bson.NewRegistryBuilder().
		RegisterDecoder(reflect.TypeOf(""), nullawareStrDecoder{}).
		Build()))

	failOnError(err, "Error while getting Mongo collection")
	return mongoClient.Database(dbName).Collection(collectionName)
}

func makeTempPL(query string) mongo.Pipeline {
	stages := mongo.Pipeline{}
	//stages = append(stages, bson.D{{"$match", bson.M{"$text": bson.M{"$search": query}}}})
	stages = append(stages, bson.D{{"$search", bson.M{"autocomplete": bson.M{"path": "metadata.title", "query": query, "tokenOrder": "sequential"}}}})
	stages = append(stages, bson.D{{"$limit", 20}})
	stages = append(stages, bson.D{{"$match", bson.M{"content.publishstate": bson.M{"$ne": false}}}})
	stages = append(stages, bson.D{{"$sort", bson.M{"score": bson.M{"$meta": "textScore"}}}})
	stages = append(stages, bson.D{{"$project", bson.M{
		"_id":          0,
		"title":        "$metadata.title",
		"poster":       "$posters.landscape",
		"portriat":     "$posters.portrait",
		"video":        "$media.video",
		"type":         "$tiletype",
		"isDetailPage": "$content.detailPage",
		"contentId":    "$ref_id",
		"package":      "$content.package",
		"source":       "$content.source",
		"contenttype":  "$content.type",
		"target":       "$content.target",
	}}})
	return stages
}

func makeTextPL(query string) mongo.Pipeline {
	pipeline := mongo.Pipeline{}
	stage1 := bson.D{{"$search", bson.M{"autocomplete": bson.M{"path": "metadata.title", "query": query}}}}
	pipeline = append(pipeline, stage1)
	stage4 := bson.D{{"$match", bson.M{"content.publishstate": bson.M{"$eq": true}}}}
	pipeline = append(pipeline, stage4)
	stage2 := bson.D{{"$limit", 10}}
	pipeline = append(pipeline, stage2)
	stage3 := bson.D{{"$project", bson.M{"_id": 0, "title": "$metadata.title"}}}
	pipeline = append(pipeline, stage3)
	return pipeline
}

func makeSugPL(query string) mongo.Pipeline {
	// creating pipes for mongo aggregation for recommedation
	stages := mongo.Pipeline{}
	//stages = append(stages, bson.D{{"$match", bson.M{"$text": bson.M{"$search": query}}}})

	stages = append(stages, bson.D{{"$search", bson.M{"autocomplete": bson.M{"path": "metadata.title", "query": query}}}})
	stages = append(stages, bson.D{{"$match", bson.M{"content.publishstate": bson.M{"$eq": true}}}})

	stages = append(stages, bson.D{{"$limit", 20}})
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

func makeOptiSugPL(query *cloudwalker.SearchQuery) mongo.Pipeline {
	// creating pipes for mongo aggregation for recommedation
	stages := mongo.Pipeline{}

	if len(query.SearchMeta) == 0 {

		stage1 := bson.D{{
			Key: "$search",
			Value: bson.D{{
				Key: "text",
				Value: bson.M{
					"query": query.GetQuery(),
					"path":  "metadata.title",
				},
			}},
		}}

		stages = append(stages, stage1)
		print("FINAL 2===>", stage1.Map())

	} else {

		filterArray := primitive.A{}

		if len(query.GetQuery()) > 0 {
			tmp := bson.D{{
				Key: "text",
				Value: bson.M{
					"query": query.GetQuery(),
					"path":  "metadata.title",
				},
			}}

			filterArray = append(filterArray, tmp)
		}

		for k, v := range query.SearchMeta {
			tmp := bson.D{{
				Key: "text",
				Value: bson.M{
					"query": v.GetValues(),
					"path":  k,
				},
			}}
			filterArray = append(filterArray, tmp)
		}

		stage1 := bson.D{{
			Key: "$search",
			Value: bson.D{{
				Key: "compound",
				Value: bson.D{{
					Key:   "must",
					Value: filterArray,
				}},
			}},
		}}

		print("FINAL 1===>", stage1.Map())

		stages = append(stages, stage1)
	}

	stages = append(stages, bson.D{{"$limit", 30}})

	return stages
}

func showSearchTextPL(query *cloudwalker.SearchQuery) mongo.Pipeline {
	stages := mongo.Pipeline{}
	//making movie search PL

	searchStage := bson.D{{
		Key: "$search",
		Value: bson.D{{
			Key: "autocomplete",
			Value: bson.M{
				"query":      query.GetQuery(),
				"path":       "showtitle",
				"score":      bson.M{"boost": bson.M{"value": 3}},
				"tokenOrder": "sequential",
				"fuzzy": bson.M{
					"maxEdits":     1,
					"prefixLength": 4,
				},
			},
		}},
	}}

	projectStage := bson.D{{
		Key: "$project",
		Value: bson.M{
			"_id":   0,
			"title": "$showtitle",
		},
	}}

	stages = append(stages, searchStage)
	// stages = append(stages, bson.D{{Key: "$sort", Value: bson.M{"score": bson.M{"$meta": "searchScore"}}}})
	stages = append(stages, bson.D{{Key: "$limit", Value: 20}})
	stages = append(stages, projectStage)
	return stages
}

func movieSearchTextPL(query *cloudwalker.SearchQuery) mongo.Pipeline {
	stages := mongo.Pipeline{}
	//making movie search PL

	searchStage := bson.D{{
		Key: "$search",
		Value: bson.D{{
			Key: "text",
			Value: bson.M{
				"query": query.GetQuery(),
				"path":  "metadata.title",
				"score": bson.M{"boost": bson.M{"value": 3}},
				"fuzzy": bson.M{
					"maxEdits":     1,
					"prefixLength": 4,
				},
			},
		}},
	}}

	projectStage := bson.D{{
		Key: "$project",
		Value: bson.M{
			"_id":   0,
			"title": "$metadata.title",
		},
	}}

	stages = append(stages, searchStage)
	// stages = append(stages, bson.D{{Key: "$sort", Value: bson.M{"score": bson.M{"$meta": "searchScore"}}}})
	stages = append(stages, bson.D{{Key: "$limit", Value: 20}})
	stages = append(stages, projectStage)
	return stages
}

func showSearchPL(query *cloudwalker.SearchQuery) mongo.Pipeline {

	println("pipe line 1 ")
	stages := mongo.Pipeline{}
	//making movie search PL
	searchStage := bson.D{}

	if len(query.SearchMeta) == 0 {
		//if search Meta Map is not present
		println("pipe line 2 ")
		searchStage = bson.D{{
			Key: "$search",
			Value: bson.D{{
				Key: "autocomplete",
				Value: bson.M{
					"query":      query.GetQuery(),
					"path":       "showtitle",
					"score":      bson.M{"boost": bson.M{"value": 3}},
					"tokenOrder": "sequential",
					"fuzzy": bson.M{
						"maxEdits":     1,
						"prefixLength": 4,
					},
				},
			}},
		}}

		stages = append(stages, searchStage)

	} else {
		//if search Meta Map is present
		filterArray := primitive.A{}
		println("pipe line 3 ")
		// adding query
		if len(query.GetQuery()) > 0 {
			println("pipe line 4 ")
			tmp := bson.D{{
				Key: "autocomplete",
				Value: bson.M{
					"query":      query.GetQuery(),
					"path":       "showtitle",
					"score":      bson.M{"boost": bson.M{"value": 3}},
					"tokenOrder": "sequential",
					"fuzzy": bson.M{
						"maxEdits":     1,
						"prefixLength": 4,
					},
				},
			}}

			filterArray = append(filterArray, tmp)
		}

		validKeys := []string{"seasonnumber", "episodenumber", "source", "showtitle"}

		//adding map
		for k, v := range query.SearchMeta {
			println("pipe line 5 ", k, "   ", v.String())

			if !contains(validKeys, k) {
				println("not a valid key ", k)
				continue
			}
			if k == "metadata.sources" {
				k = "source"
			}

			tmp := bson.D{{
				Key: "text",
				Value: bson.M{
					"query": v.GetValues(),
					"path":  k,
					"fuzzy": bson.M{},
				},
			}}
			filterArray = append(filterArray, tmp)
		}

		if len(filterArray) > 0 {

			searchStage = bson.D{{
				Key: "$search",
				Value: bson.D{{
					Key: "compound",
					Value: bson.M{
						"must": filterArray,
					},
				}},
			}}

			stages = append(stages, searchStage)
		}

	}

	//lookup stage
	lookUpStage1 := bson.D{{
		Key: "$lookup",
		Value: bson.M{
			"from":         "justwatch_offers",
			"localField":   "offerrefid",
			"foreignField": "offerrefid",
			"as":           "contentavailable",
		},
	}}

	//uniwind stage
	unwindStage1 := bson.D{{
		Key:   "$unwind",
		Value: "$contentavailable",
	}}

	lookUpStage2 := bson.D{{
		Key: "$lookup",
		Value: bson.M{
			"from":         "justwatch",
			"localField":   "parentrefid",
			"foreignField": "refid",
			"as":           "details",
		},
	}}

	//uniwind stage
	unwindStage2 := bson.D{{
		Key:   "$unwind",
		Value: "$details",
	}}

	projectStage := bson.D{{
		Key: "$project",
		Value: bson.M{
			"_id":          0,
			"title":        "$details.metadata.title",
			"poster":       "$details.media.landscape",
			"portriat":     "$details.media.portrait",
			"video":        "$details.media.video",
			"type":         "$details.tiletype",
			"season":       "$seasonnumber",
			"episode":      "$episodenumber",
			"isDetailPage": "$details.content.detailpage",
			"contentId":    "$details.refid",
			"score":        bson.M{"$meta": "searchScore"},
			"play":         bson.A{"$contentavailable"},
		},
	}}

	//adding stage NOTE Sequence is IMPT
	stages = append(stages, bson.D{{Key: "$limit", Value: 10}})
	stages = append(stages, lookUpStage1)
	stages = append(stages, unwindStage1)
	stages = append(stages, lookUpStage2)
	stages = append(stages, unwindStage2)
	stages = append(stages, projectStage)

	return stages
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func movieSearchPL(query *cloudwalker.SearchQuery) mongo.Pipeline {

	stages := mongo.Pipeline{}
	//making movie search PL
	searchStage := bson.D{}

	if len(query.SearchMeta) == 0 {
		//if search Meta Map is not present
		searchStage = bson.D{{
			Key: "$search",
			Value: bson.D{{
				Key: "text",
				Value: bson.M{
					"query": query.GetQuery(),
					"path":  "metadata.title",
					"score": bson.M{"boost": bson.M{"value": 3}},
					"fuzzy": bson.M{
						"maxEdits":     1,
						"prefixLength": 4,
					},
				},
			}},
		}}

	} else {

		//if search Meta Map is present
		filterArray := primitive.A{}

		// adding query
		if len(query.GetQuery()) > 0 {
			tmp := bson.D{{
				Key: "text",
				Value: bson.M{
					"query": query.GetQuery(),
					"path":  "metadata.title",
					"score": bson.M{"boost": bson.M{"value": 3}},
					"fuzzy": bson.M{
						"maxEdits":     1,
						"prefixLength": 4,
					},
				},
			}}
			filterArray = append(filterArray, tmp)
		}

		//adding map
		for k, v := range query.SearchMeta {
			tmp := bson.D{{
				Key: "text",
				Value: bson.M{
					"query": v.GetValues(),
					"path":  k,
					"fuzzy": bson.M{},
				},
			}}
			filterArray = append(filterArray, tmp)
		}

		//adding mustNot i.e. search other than Tv Show
		mustnot := bson.D{{
			Key: "text",
			Value: bson.M{
				"query": []string{"Tv Show"},
				"path":  "metadata.categories",
			},
		}}

		searchStage = bson.D{{
			Key: "$search",
			Value: bson.D{{
				Key: "compound",
				Value: bson.M{
					"must":    filterArray,
					"mustNot": mustnot,
				},
			}},
		}}
	}

	//lookup stage
	lookUpStage1 := bson.D{{
		Key: "$lookup",
		Value: bson.M{
			"from":         "justwatch_offers",
			"localField":   "metadata.offerrefid",
			"foreignField": "offerrefid",
			"as":           "contentavailable",
		},
	}}

	projectStage := bson.D{{
		Key: "$project",
		Value: bson.M{
			"_id":          0,
			"title":        "$metadata.title",
			"poster":       "$media.landscape",
			"portriat":     "$media.portrait",
			"video":        "$media.video",
			"type":         "$tiletype",
			"isDetailPage": "$content.detailpage",
			"contentId":    "$refid",
			"score":        bson.M{"$meta": "searchScore"},
			"play":         "$contentavailable",
		},
	}}

	//adding stage NOTE Sequence is IMPT
	stages = append(stages, searchStage)
	// stages = append(stages, bson.D{{Key: "$sort", Value: bson.M{"metadata.year": -1, "score": bson.M{"$meta": "searchScore"}}}})
	stages = append(stages, bson.D{{Key: "$limit", Value: 10}})
	stages = append(stages, lookUpStage1)
	stages = append(stages, projectStage)

	return stages
}
