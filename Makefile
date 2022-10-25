export GOPATH=$(PWD)/.go/

modules := core io net run ui

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
	./$<

test:
	go test $(test_verbosity_options) -count=1 $(test_parameters)

bench:
	go test $(test_verbosity_options) -bench=. $(addprefix ./, $(T))


bin/silk: $(foreach d, $(modules), $(wildcard $(d)/*.go))
	go build -v -race -o $@ ./ui


clean:
	-rm -rf bin

cleanall: clean
	-rm -rf $(PWD)/.go/
