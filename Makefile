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
libsonnetFiles = $(shell find $(libsonnetDir) -name '*.libsonnet' -maxdepth 1 -type f)
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


all: $(specfile) git-tag $(executables) $(buildTarPath) $(NAME)-$(rpmVersion)*.rpm
.PHONY: all all-test git-tag git-revert-tag clean clean-all


$(specfile): Makefile
	sed -Ei 's/Version\:[ ]*.+/Version:        $(rpmVersion)/' $(specfile)
	echo "Set version in spec file to $(rpmVersion)"


git-tag: $(specfile)
	git add Makefile $(specfile)
	git commit $(signflagCommit) -m 'Release $(VERSION)'
	git tag $(VERSION)


$(executables): cmd/*/*.go internal/*/*.go
	for exe in $(executables); do \
		echo "Building $$exe"; \
		cd cmd/$$exe;\
		go build $(raceflag) -ldflags="-X main.buildTimestamp=$(BUILD)" -o $(ROOTDIR)/$$exe;  \
		echo "Built $$exe"; \
		cd $(ROOTDIR); \
	done


$(buildTarPath): $(executables) $(ROOTDIR)/libsonnet $(ROOTDIR)/Makefile_jsonnet $(ROOTDIR)/managedTokens.jsonnet $(ROOTDIR)/packaging/*
	mkdir -p $(SOURCEDIR)
	mkdir -p $(SOURCEDIR)/libsonnet
	cp $(foreach exe,$(executables),$(ROOTDIR)/$(exe)) $(SOURCEDIR)  # Executables
	cp $(ROOTDIR)/managedTokens.jsonnet $(ROOTDIR)/Makefile_jsonnet $(ROOTDIR)/packaging/managed-tokens.logrotate $(ROOTDIR)/packaging/managed-tokens.cron $(SOURCEDIR)  # Config files
	cp $(foreach lsfile,$(libsonnetFiles),$(lsfile)) $(SOURCEDIR)/libsonnet/$(notdir $(lsfile)) # Libsonnet files
	tar -czf $(buildTarPath) -C $(ROOTDIR) $(buildTarName)
	echo "Built deployment tarball"


$(NAME)-$(rpmVersion)*.rpm: rpmSourcesDir := $$HOME/rpmbuild/SOURCES
$(NAME)-$(rpmVersion)*.rpm: rpmSpecsDir := $$HOME/rpmbuild/SPECS
$(NAME)-$(rpmVersion)*.rpm: rpmDir := $$HOME/rpmbuild/RPMS/x86_64/
$(NAME)-$(rpmVersion)*.rpm: $(specfile) $(buildTarPath)
	cp $< $(rpmSpecsDir)/
	cp $(buildTarPath) $(rpmSourcesDir)/
	cd $(rpmSpecsDir); \
	rpmbuild -ba ${NAME}.spec
	find $(HOME)/rpmbuild/RPMS -type f -name "$(NAME)-$(rpmVersion)*.rpm" -cmin 1 -exec cp {} $(ROOTDIR)/ \;
	echo "Created RPM and copied it to current working directory"

podman-test: all
	podman build -t managed-tokens-test . --build-arg=rpmfile=$(shell find $(ROOTDIR) -maxdepth 1 -type f -name "$(NAME)-$(rpmVersion)*.rpm" | head -n 1 | xargs basename)
	podman run --rm managed-tokens-test
	echo "Docker test completed"

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
	(test -e $(ROOTDIR)/$(NAME)-$(rpmVersion)*.rpm) && (rm $(ROOTDIR)/$(NAME)-$(rpmVersion)*.rpm)
