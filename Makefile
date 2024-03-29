BIN := bin/


export GOPATH=$(PWD)/.go/


modules := io kv net run ui util/atomic util/rand
mains := kv main run send server


T ?= $(modules)
V := 1


ifeq ($(V),2)
  test_verbosity_options := -v
endif

test_parameters := $(addprefix ./, $(filter $(modules), $(T))) \
                   $(patsubst %, -run='%', $(filter-out $(modules), $(T)))

ifeq ($(filter $(modules), $(T)),)
  test_parameters := $(addprefix ./, $(modules)) $(test_parameters)
endif


-include .config/Makefile


all: $(BIN)silk


install: $(BIN)silk
	mkdir -p $(prefix)/bin
	cp $< $(prefix)/bin/silk


test: unit-test validation-test

unit-test:
	ulimit -n $$(ulimit -Hn) && \
        go test $(test_verbosity_options) -count=1 $(test_parameters)

validation-test: $(BIN)silk
	./tool/validate -p $(BIN)

bench:
	go test $(test_verbosity_options) -bench=. $(addprefix ./, $(T))


$(BIN)silk: $(addsuffix .go, $(mains)) \
          $(foreach d, $(modules), $(wildcard $(d)/*.go)) | $(BIN)
	go build -v -o $@ $(addsuffix .go, $(mains))


$(BIN):
	mkdir $(BIN)


clean:
	-rm -rf $(BIN)

cleanall: clean
	-chmod -R u+wx $(PWD)/.go/
	-rm -rf $(PWD)/.go/
