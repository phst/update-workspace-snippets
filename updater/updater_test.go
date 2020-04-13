// Copyright 2020 Google LLC
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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

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
		t.Error(err)
	}
	defer os.RemoveAll(tempDir)

	worktreeDir := filepath.Join(tempDir, "worktree")
	repo, worktree := initLocalRepo(t, worktreeDir)

	remoteDir := filepath.Join(tempDir, "remote.git")
	initRemoteRepo(t, remoteDir)

	const before = `To use this repository, add the following to your WORKSPACE file:

  http_archive(
      name = "foo",
      urls = ["http://archive/.zip"],
      sha256sum = "",
      strip_prefix = "repo-",
  )

Have a nice day!`
	readme := filepath.Join(worktreeDir, "README")
	write(t, readme, before)
	hash := commit(t, worktree)
	push(t, repo, "github", remoteDir)

	archiveDir := filepath.Join(remoteDir, "archive")
	mkdir(t, archiveDir)
	archiveFile := filepath.Join(archiveDir, hash.String()+".zip")
	write(t, archiveFile, "archive contents\n")

	transport := new(http.Transport)
	transport.RegisterProtocol("", http.NewFileTransport(http.Dir("/")))
	client := &http.Client{Transport: transport}

	u, err := updater.New(worktreeDir, client, remoteDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := u.Update(readme); err != nil {
		t.Errorf("error updating %s: %s", readme, err)
	}

	got := read(t, readme)
	want := fmt.Sprintf(
		`To use this repository, add the following to your WORKSPACE file:

  http_archive(
      name = "foo",
      urls = ["http://archive/%[1]s.zip"],
      sha256sum = "d3c33415db7cef081c5b86d1f1822a056b93a98faf37a7f7e92791f677f3a3c2",
      strip_prefix = "repo-%[1]s",
  )

Have a nice day!`, hash)

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
		t.Error(err)
	}
}

func commit(t *testing.T, worktree *git.Worktree) plumbing.Hash {
	hash, err := worktree.Commit("commit message", &git.CommitOptions{All: true, Author: new(object.Signature)})
	if err != nil {
		t.Error(err)
	}
	return hash
}

func push(t *testing.T, repo *git.Repository, remoteName, remoteDir string) {
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: remoteName, URLs: []string{remoteDir}}); err != nil {
		t.Error(err)
	}
	if err := repo.Push(&git.PushOptions{RemoteName: remoteName}); err != nil {
		t.Error(err)
	}
}

func mkdir(t *testing.T, dir string) {
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Error(err)
	}
}

func read(t *testing.T, name string) string {
	b, err := ioutil.ReadFile(name)
	if err != nil {
		t.Error(err)
	}
	return string(b)
}

func write(t *testing.T, name, contents string) {
	if err := ioutil.WriteFile(name, []byte(contents), 0600); err != nil {
		t.Error(err)
	}
}
