# Copyright 2025 Philipp Stephani
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.POSIX:
.SUFFIXES:

SHELL = /bin/sh

BAZEL = bazel
GO = go
STATICCHECK = staticcheck

all:
	$(BAZEL) build $(BAZELFLAGS) -- //...

check: all
	$(BAZEL) test $(BAZELFLAGS) -- //...
	$(BAZEL) run $(BAZELFLAGS) -- \
	  @com_github_google_addlicense//:addlicense \
	  -check -- "$${PWD}"
	$(BAZEL) run $(BAZELFLAGS) -- \
	  @buildifier_prebuilt//:buildifier \
	  -mode=check -lint=warn -warnings=all -r -- "$${PWD}"
	$(GO) vet ./...
	$(STATICCHECK) ./...

install:
