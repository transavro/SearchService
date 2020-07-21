INPUT_DIR = proto
INPUT_PATH = ${INPUT_DIR}/CDEService.proto
GOPATH = /home/nayan/go
GOOGLE_APIS_PATH = ${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis
OUTPUT_PATH = gen


grpc_gen:
	protoc -I${INPUT_DIR} -I${GOPATH}/src -I${GOOGLE_APIS_PATH} --go_out=plugins=grpc:${OUTPUT_PATH} ${INPUT_PATH}

gateway_gen:
	protoc -I${INPUT_DIR} -I${GOPATH}/src -I${GOOGLE_APIS_PATH} --grpc-gateway_out=logtostderr=true:${OUTPUT_PATH} ${INPUT_PATH}

swagger_gen:
	protoc -I${INPUT_DIR} -I${GOPATH}/src -I${GOOGLE_APIS_PATH} --swagger_out=logtostderr=true:${OUTPUT_PATH} ${INPUT_PATH}


proto_gen: make_output_dir proto_build

proto_build: grpc_gen gateway_gen swagger_gen

make_output_dir : 
	mkdir ${OUTPUT_PATH}

clean:
	rm ${OUTPUT_PATH}/* 

build_run:
	go build .
	./SearchService

