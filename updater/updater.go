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

package updater

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Updater updates Bazel workspace snippets.  The zero Updater is not valid;
// use New to create Updater objects.
type Updater struct {
	refHash     plumbing.Hash
	archiveHash archiveHash
}

// New creates a new Updater.  dir must be a directory within a checked-out Git
// repository.  The repository must have exactly one remote whose URL starts
// with urlPrefix (normally https://github.com/).  The remote must support
// GitHub’s archive functionality.
func New(dir string, client *http.Client, urlPrefix string) (*Updater, error) {
	if urlPrefix == "" {
		return nil, errors.New("updater: empty URL prefix")
	}
	if dir == "" {
		return nil, errors.New("updater: empty directory")
	}
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("updater: can’t open Git repository in %s: %w", dir, err)
	}
	remote, url, err := remote(repo, urlPrefix)
	if err != nil {
		return nil, fmt.Errorf("updater: no GitHub remote for Git repository in %s: %w", dir, err)
	}
	refHash, err := masterHash(remote)
	if err != nil {
		return nil, fmt.Errorf("updater: no remote hash for Git repository in %s: %w", dir, err)
	}
	// The archive URL doesn’t work with the .git suffix.
	archiveURL := strings.TrimSuffix(url, ".git") + "/archive/" + refHash.String() + ".zip"
	archiveHash, err := hashArchive(client, archiveURL)
	if err != nil {
		return nil, fmt.Errorf("updater: can’t download archive for Git repository in %s: %w", dir, err)
	}
	return &Updater{refHash, archiveHash}, nil
}

// Update updates commit and archive hashes within the given file.
// The file must contain at least one stanza of the form
//
//   http_archive(
//       name = "…",
//       urls = ["https://github.com/owner/repo/archive/〈hash〉.zip"],
//       sha256sum = "…",
//       strip_prefix = "repo-〈hash〉",
//   )
//
// Update replaces the hashes with the values from the upstream HEAD commit.
func (u *Updater) Update(file string) error {
	contents, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("updater: can’t read file: %w", err)
	}

	var out bytes.Buffer

	found := false
	for {
		indices := beginPattern.FindSubmatchIndex(contents)
		if indices == nil {
			if !found {
				return fmt.Errorf("updater: no http_archive stanza in file %s", file)
			}
			break
		}
		found = true
		out.Write(contents[:indices[1]])
		prefix := regexp.QuoteMeta(string(contents[indices[2]:indices[3]]))
		contents = contents[indices[1]:]

		endPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^%s[ \t]*\)$`, prefix))
		indices = endPattern.FindIndex(contents)
		if indices == nil {
			return fmt.Errorf("updater: http_stanza in file %s not properly terminated", file)
		}
		slice := contents[:indices[0]]
		out.Write(u.update(slice, prefix))

		out.Write(contents[indices[0]:indices[1]])
		contents = contents[indices[1]:]
	}

	out.Write(contents)

	temp := file + ".tmp"
	if err := ioutil.WriteFile(temp, out.Bytes(), 0600); err != nil {
		return fmt.Errorf("update: can’t write temporary output file: %w", err)
	}
	if err := os.Rename(temp, file); err != nil {
		return fmt.Errorf("update: can’t rename temporary file: %w", err)
	}
	return nil
}

var beginPattern = regexp.MustCompile(`(?m)^([ \t]*(?://+|#+)?[ \t]*)http_archive\($`)

func (u *Updater) update(b []byte, p string) []byte {
	urlsPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^(%s[ \t]*urls = \[".+/)[[:xdigit:]]*(\.zip"\],)$`, p))
	archiveHashPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^(%s[ \t]*sha256sum = ")[[:xdigit:]]*(",)$`, p))
	stripPrefixPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^(%s[ \t]*strip_prefix = ".*)[[:xdigit:]]*(",)$`, p))

	refHashRepl := []byte(fmt.Sprintf("${1}%s${2}", u.refHash))
	archiveHashRepl := []byte(fmt.Sprintf("${1}%s${2}", u.archiveHash))

	b = urlsPattern.ReplaceAll(b, refHashRepl)
	b = archiveHashPattern.ReplaceAll(b, archiveHashRepl)
	b = stripPrefixPattern.ReplaceAll(b, refHashRepl)
	return b
}

func remote(repo *git.Repository, prefix string) (remote *git.Remote, url string, err error) {
	all, err := repo.Remotes()
	if err != nil {
		return nil, "", err
	}
	for _, r := range all {
		u := matchingURL(r, prefix)
		if u == "" {
			continue
		}
		if remote != nil {
			return nil, "", errors.New("multiple GitHub remotes found")
		}
		remote, url = r, u
	}
	if remote == nil {
		return nil, "", errors.New("no GitHub remote found")
	}
	return
}

func matchingURL(remote *git.Remote, prefix string) string {
	for _, url := range remote.Config().URLs {
		if strings.HasPrefix(url, prefix) {
			return url
		}
	}
	return ""
}

func masterHash(remote *git.Remote) (plumbing.Hash, error) {
	refs, err := remote.List(new(git.ListOptions))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("can’t list remote %s: %w", remote, err)
	}
	for _, r := range refs {
		if r.Name() == plumbing.Master && r.Type() == plumbing.HashReference {
			return r.Hash(), nil
		}
	}
	return plumbing.ZeroHash, fmt.Errorf("no master reference in remote %s found", remote)
}

func hashArchive(client *http.Client, url string) (archiveHash, error) {
	var r archiveHash
	resp, err := client.Get(url)
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return r, fmt.Errorf("downloading %s resulting in HTTP status %s", url, resp.Status)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, resp.Body); err != nil {
		return r, fmt.Errorf("couldn’t download %s: %w", url, err)
	}
	if err := resp.Body.Close(); err != nil {
		return r, fmt.Errorf("couldn’t download %s: %w", url, err)
	}
	s := hash.Sum(nil)
	if len(s) != len(r) {
		return r, fmt.Errorf("invalid hash size: got %d, want %d", len(s), len(r))
	}
	copy(r[:], s)
	return r, nil
}

type archiveHash [sha256.Size]byte

func (h archiveHash) String() string {
	return hex.EncodeToString(h[:])
}
