# Copyright 2023, 2025 Philipp Stephani
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

load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "updater",
    srcs = ["updater.go"],
    importpath = "github.com/phst/update-workspace-snippets/updater",
    visibility = ["//:__pkg__"],
    deps = [
        "@com_github_bazelbuild_buildtools//build",
        "@com_github_go_git_go_git_v5//:go-git",
        "@com_github_go_git_go_git_v5//plumbing",
    ],
)

go_test(
    name = "updater_test",
    size = "medium",
    timeout = "short",
    srcs = ["updater_test.go"],
    deps = [
        ":updater",
        "@com_github_go_git_go_git_v5//:go-git",
        "@com_github_go_git_go_git_v5//config",
        "@com_github_go_git_go_git_v5//plumbing",
        "@com_github_go_git_go_git_v5//plumbing/object",
        "@com_github_google_go_cmp//cmp",
    ],
)
