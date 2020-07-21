package main

import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"reflect"
	"time"
)

type nullawareStrDecoder struct{}

func failOnError(err error, msg string){
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
	stages = append(stages, bson.D{{"$search", bson.M{"autocomplete" : bson.M{"path":"metadata.title","query":query}}}} )
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
	stage1 := bson.D{{"$search", bson.M{"autocomplete" : bson.M{"path":"metadata.title","query":query}}}}
	pipeline = append(pipeline, stage1)
	stage2 := bson.D{{"$limit", 10}}
	pipeline = append(pipeline, stage2)
	stage3 := bson.D{{"$project", bson.M{"_id":0,"title": "$metadata.title"}}}
	pipeline = append(pipeline, stage3)
	return pipeline
}

func makeSugPL(query string) mongo.Pipeline {
	// creating pipes for mongo aggregation for recommedation
	stages := mongo.Pipeline{}
	//stages = append(stages, bson.D{{"$match", bson.M{"$text": bson.M{"$search": query}}}})

	stages = append(stages, bson.D{{"$search", bson.M{"autocomplete" : bson.M{"path":"metadata.title","query":query}}}})
	stages = append(stages, bson.D{{"$limit", 20}})
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