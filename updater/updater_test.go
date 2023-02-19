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

package updater_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-cmp/cmp"
	"github.com/phst/update-workspace-snippets/updater"
)

func TestUpdater(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "updater-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	worktreeDir := filepath.Join(tempDir, "worktree")
	repo, worktree := initLocalRepo(t, worktreeDir)

	remoteDir := filepath.Join(tempDir, "remote.git")
	initRemoteRepo(t, remoteDir)

	const before = `To use this repository, add the following to your WORKSPACE file:

  http_archive(
      name = "foo",
      urls = [
          "http://archive/.zip",   # 2020-07-06
      ],
      sha256 = "",
      strip_prefix = "repo-1234",
  )

Have a nice day!`
	readme := filepath.Join(worktreeDir, "README")
	write(t, readme, before)
	commitHash := commit(t, worktree)
	push(t, repo, "github", remoteDir)

	b := new(bytes.Buffer)
	w := zip.NewWriter(b)
	now := time.Date(2021, time.April, 24, 17, 32, 22, 0, time.UTC)
	if _, err := w.CreateHeader(&zip.FileHeader{Name: "root/", Modified: now}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	archiveHash := sha256.Sum256(b.Bytes())

	archiveDir := filepath.Join(tempDir, "remote", "archive")
	mkdir(t, archiveDir)
	archiveFile := filepath.Join(archiveDir, commitHash.String()+".zip")
	write(t, archiveFile, b.String())

	transport := new(http.Transport)
	transport.RegisterProtocol("", http.NewFileTransport(http.Dir("/")))
	client := &http.Client{Transport: transport}

	u, err := updater.New(worktreeDir, client, remoteDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := u.Update(readme); err != nil {
		t.Fatalf("error updating %s: %s", readme, err)
	}

	got := read(t, readme)
	want := fmt.Sprintf(
		`To use this repository, add the following to your WORKSPACE file:

  http_archive(
      name = "foo",
      urls = [
          "http://archive/%[1]s.zip",  # 2021-04-24
      ],
      sha256 = "%[2]x",
      strip_prefix = "repo-%[1]s",
  )

Have a nice day!`, commitHash, archiveHash)

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("file %s: -got +want:\n%s", readme, diff)
	}
}

func initLocalRepo(t *testing.T, dir string) (*git.Repository, *git.Worktree) {
	mkdir(t, dir)
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	return repo, worktree
}

func initRemoteRepo(t *testing.T, dir string) {
	mkdir(t, dir)
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, worktree *git.Worktree) plumbing.Hash {
	hash, err := worktree.Commit("commit message", &git.CommitOptions{All: true, Author: new(object.Signature)})
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func push(t *testing.T, repo *git.Repository, remoteName, remoteDir string) {
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: remoteName, URLs: []string{remoteDir}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Push(&git.PushOptions{RemoteName: remoteName}); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, dir string) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, name string) string {
	b, err := ioutil.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, name, contents string) {
	if err := ioutil.WriteFile(name, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}
