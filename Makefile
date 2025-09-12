NAME = managed-tokens
VERSION = v0.17.0
ROOTDIR = $(shell pwd)
BUILD = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
rpmVersion := $(subst v,,$(VERSION))
buildTarName = $(NAME)-$(rpmVersion)
buildTarPath = $(ROOTDIR)/$(buildTarName).tar.gz
SOURCEDIR = $(ROOTDIR)/$(buildTarName)
executables = refresh-uids-from-ferry token-push
libsonnetDir = $(ROOTDIR)/libsonnet
libsonnetFiles = $(shell find $(libsonnetDir) -maxdepth 1  -name '*.libsonnet' -type f)
specfile := $(ROOTDIR)/packaging/$(NAME).spec
ifdef RACE
raceflag := -race
else
raceflag :=
endif
ifdef SIGN
signflagCommit := -S
else
signflagCommit :=
endif


all: test release podman-test
release: $(specfile) git-tag $(executables) $(buildTarPath) $(NAME)-$(rpmVersion)-*.rpm
.PHONY: all release test git-tag podman-test git-revert-tag clean clean-all

test:
	go test -v ./... && echo "All tests passed" || (echo "Some tests failed" && exit 1)

$(specfile): Makefile
	sed -Ei 's/Version\:[ ]*.+/Version:        $(rpmVersion)/' $(specfile)
	echo "Set version in spec file to $(rpmVersion)"


git-tag: $(specfile)
	git add Makefile $(specfile)
	git commit $(signflagCommit) -m 'Release $(VERSION)'
	git tag $(VERSION)


$(executables): cmd/*/*.go internal/*/*.go
    mkdir $(ROOTDIR)/binbackup
	for exe in $(executables); do \
		echo "Backing up existing $$exe if it exists"; \
		(test -e $(ROOTDIR)/$$exe) && (mv $(ROOTDIR)/$$exe $(ROOTDIR)/binbackup/$$exe); \
		echo "Building $$exe"; \
		cd cmd/$$exe; \
		go build $(raceflag) -ldflags="-X main.buildTimestamp=$(BUILD)" -o $(ROOTDIR)/$$exe;  \
		echo "Built $$exe"; \
		cd $(ROOTDIR); \
	done
	rm -Rf $(ROOTDIR)/binbackup


$(buildTarPath): $(executables) $(ROOTDIR)/libsonnet $(ROOTDIR)/Makefile_jsonnet $(ROOTDIR)/managedTokens.jsonnet $(ROOTDIR)/packaging/*
	mkdir -p $(SOURCEDIR)
	mkdir -p $(SOURCEDIR)/libsonnet
	cp $(foreach exe,$(executables),$(ROOTDIR)/$(exe)) $(SOURCEDIR)  # Executables
	cp $(ROOTDIR)/managedTokens.jsonnet $(ROOTDIR)/Makefile_jsonnet $(ROOTDIR)/packaging/managed-tokens.logrotate $(ROOTDIR)/packaging/managed-tokens.cron $(SOURCEDIR)  # Config files
	cp $(foreach lsfile,$(libsonnetFiles),$(lsfile)) $(SOURCEDIR)/libsonnet/$(notdir $(lsfile)) # Libsonnet files
	tar -czf $(buildTarPath) -C $(ROOTDIR) $(buildTarName)
	echo "Built deployment tarball"


$(NAME)-$(rpmVersion)-*.rpm: rpmSourcesDir := $$HOME/rpmbuild/SOURCES
$(NAME)-$(rpmVersion)-*.rpm: rpmSpecsDir := $$HOME/rpmbuild/SPECS
$(NAME)-$(rpmVersion)-*.rpm: rpmDir := $$HOME/rpmbuild/RPMS/x86_64/
$(NAME)-$(rpmVersion)-*.rpm: $(specfile) $(buildTarPath)
	cp $< $(rpmSpecsDir)/
	cp $(buildTarPath) $(rpmSourcesDir)/
	cd $(rpmSpecsDir); \
	rpmbuild -ba ${NAME}.spec
	find $(HOME)/rpmbuild/RPMS -type f -name "$@" -cmin 1 -exec cp {} $(ROOTDIR)/ \;
	echo "Created RPM and copied it to current working directory"


podman-test:
	podman build -f $(ROOTDIR)/packaging/Dockerfile_test -t managed-tokens-test --build-arg=rpmfile=$(shell find $(ROOTDIR) -maxdepth 1 -type f -name "$(NAME)-$(rpmVersion)*.rpm" | head -n 1 | xargs basename) .
	# We want to see if the version inside the container matches the built version
	[ "$$(podman run --rm managed-tokens-test | cut -f 1 -d ',' | cut -f 5 -d ' ')" = "$(VERSION)" ] && echo "Podman test passed" || (echo "Podman test failed")
	podman image rm managed-tokens-test

git-revert-tag:
	git tag -d $(VERSION)
	git reset --hard HEAD~1
	echo "Reverted git tag and commit for version $(VERSION)"

clean: rpmSourcesDir := $$HOME/rpmbuild/SOURCES
clean:
	for exe in $(executables); do \
		(test -e $(ROOTDIR)/$$exe) && (rm $(ROOTDIR)/$$exe); \
	done
	(test -e $(buildTarPath)) && (rm $(buildTarPath))
	(test -e $(SOURCEDIR)) && (rm -Rf $(SOURCEDIR))
	(test -e $(rpmSourcesDir)/$(buildTarName).tar.gz) && (rm $(rpmSourcesDir)/$(buildTarName).tar.gz)

clean-all: clean git-revert-tag
	(test -e $(ROOTDIR)/$(NAME)-$(rpmVersion)-*.rpm) && (rm $(ROOTDIR)/$(NAME)-$(rpmVersion)-*.rpm)
