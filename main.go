// Copyright 2020, 2021, 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Binary update-workspace-snippets automatically updates snippets for Bazel
// WORKSPACE and MODULE.bazel files.  These snippets should follow the format
// described in
// https://docs.bazel.build/versions/3.0.0/skylark/deploying.html#readme.
//
// # Usage
//
// A command like
//
//	update-workspace-snippets filename…
//
// updates the specified files so that the workspace and module snippets in
// these files point to the latest commit available on GitHub.  Right now this
// only supports commits on the master branch; releases aren’t supported yet.
// The current directory must be in a Git repository that has exactly one
// remote pointing to a GitHub repository.  The listed files should contain at
// least one “git_override” or “http_archive” stanza pointing to the GitHub
// repository.  The program also updates comments within the stanza that look
// like dates.
//
// Invoke this program after pushing to GitHub.  Since it modifies the
// workspace, you’ll typically need to commit and push again, but since it only
// updates documentation, pointing to the previous commit is fine.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/phst/update-workspace-snippets/updater"
)

func main() {
	log.SetFlags(0)
	flag.Parse()
	if flag.NArg() == 0 {
		log.Fatal("no files given")
	}
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	u, err := updater.New(dir, http.DefaultClient, "https://github.com/")
	if err != nil {
		log.Fatal(err)
	}
	success := true
	for _, file := range flag.Args() {
		if err := u.Update(file); err != nil {
			log.Print(err)
			success = false
		}
	}
	if !success {
		os.Exit(1)
	}
}
