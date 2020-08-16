APPNAME = coolapp
BLDDIR = build
BLDFLAGS=
# extension if windows
EXT=
ifeq (${GOOS},windows)
	EXT=.exe
endif

all: $(APPNAME)

$(BLDDIR)/$(APPNAME): 	$(wildcard *.go **/*.go)
	@mkdir -p $(dir $@)
	go build ${BLDFLAGS} -o $@ 

$(APPNAME) : %: $(BLDDIR)/%

test:
	go test .

clean:
	rm -rf $(BLDDIR)

.PHONY: clean test all

.PHONY: $(APPNAME)
