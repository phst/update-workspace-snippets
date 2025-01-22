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

load("@rules_go//go:def.bzl", "go_binary", "go_library")

go_binary(
    name = "update-workspace-snippets",
    embed = [":lib"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "lib",
    srcs = ["main.go"],
    importpath = "github.com/phst/update-workspace-snippets",
    visibility = ["//visibility:private"],
    deps = ["//updater"],
)

exports_files(
    ["MODULE.bazel"],
    visibility = ["//dev:__pkg__"],
)
