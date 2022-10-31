export GOPATH=$(PWD)/.go/

modules := io net run ui
mains := main run server

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


all: bin/silk

check: bin/silk
	./$< run "localhost:3200,(localhost:3201|localhost:3202)" ls

test:
	go test $(test_verbosity_options) -count=1 $(test_parameters)

bench:
	go test $(test_verbosity_options) -bench=. $(addprefix ./, $(T))


bin/silk: $(addsuffix .go, $(mains)) \
          $(foreach d, $(modules), $(wildcard $(d)/*.go))
	go build -v -race -o $@ $(addsuffix .go, $(mains))


clean:
	-rm -rf bin

cleanall: clean
	-rm -rf $(PWD)/.go/
