PATH  := node_modules/.bin:$(PATH)
branch := $(GIT_BRANCH_FOR_MAKE)
now := $(shell date '+%Y%m%d%H%M%S')
EMPTY :=
projectName := consul-balancer

protoPath := proto

ifeq ($(branch),$(EMPTY))
	branch := test
endif


all:

check:
	
switch2tag:
	python scripts/switch_tag.py

# 需要安装依赖 sudo apt install graphviz -y
# go get github.com/kisielk/godepgraph
dep:
	godepgraph -s github.com/yunnysunny/$(projectName) | dot -Tpng -o ../coverage/godepgraph.png

gen-proto:
	protoc --go_out=./   \
	--go-grpc_out=./  \
	--go_opt=paths=import \
	--go-grpc_opt=paths=import \
	--go_opt=Mhealth/health.proto=./grpc_health_v1 \
	--go-grpc_opt=Mhealth/health.proto=./grpc_health_v1 \
	health/health.proto


pull:check
	git checkout $(branch) && git pull origin $(branch)

test:check
	mkdir -p coverage && \
	go test ./... -v -timeout 200s  -cover -coverprofile=./coverage/coverage.out

coverage:test
	go tool cover -func ./coverage/coverage.out && \
	go tool cover -html ./coverage/coverage.out -o ./coverage/index.html

run:check
	
clean:
	rm -rf bin/*

grace:pull test

.PHONY: check pull test coverage run clean dep gen-proto grace
