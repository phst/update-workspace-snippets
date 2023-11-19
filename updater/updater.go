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

// Package updater contains the type [Updater] to update Bazel workspace and
// module snippets.
package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bazelbuild/buildtools/build"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Updater updates Bazel workspace and module snippets.  The zero Updater is
// not valid; use [New] to create Updater objects.
type Updater struct {
	refHash     plumbing.Hash
	archiveHash archiveHash
	date        string
}

// New creates a new [Updater].  dir must be a directory within a checked-out
// Git repository.  The repository must have exactly one remote whose URL
// starts with urlPrefix (normally https://github.com/).  The remote must
// support GitHub’s archive functionality.
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
	archiveHash, modified, err := downloadArchive(client, archiveURL)
	if err != nil {
		return nil, fmt.Errorf("updater: can’t download archive for Git repository in %s: %w", dir, err)
	}
	return &Updater{refHash, archiveHash, modified.Format("2006-01-02")}, nil
}

// Update updates commit and archive hashes within the given file.
// The file must contain at least one stanza of the form
//
//	git_override(
//	    module_name = "…",
//	    remote = "…",
//	    commit = "〈hash〉",
//	)
//
// or
//
//	http_archive(
//	    name = "…",
//	    urls = ["https://github.com/owner/repo/archive/〈hash〉.zip"],
//	    sha256 = "…",
//	    strip_prefix = "repo-〈hash〉",
//	)
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
				return fmt.Errorf("updater: no git_override or http_archive stanza in file %s", file)
			}
			break
		}
		found = true
		out.Write(contents[:indices[0]])
		prefix := string(contents[indices[2]:indices[3]])
		stanza := string(contents[indices[4]:indices[5]])
		contents = contents[indices[0]:]

		endPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^%s[ \t]*\)$`, regexp.QuoteMeta(prefix)))
		indices = endPattern.FindIndex(contents)
		if indices == nil {
			return fmt.Errorf("updater: %s stanza in file %s not properly terminated", stanza, file)
		}
		slice := contents[:indices[1]]
		out.Write(u.update(slice, prefix))

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

var beginPattern = regexp.MustCompile(`(?m)^([ \t]*(?://+|#+)?[ \t]*)(git_override|http_archive)\($`)

func (u *Updater) update(b []byte, p string) []byte {
	b = regexp.MustCompile(fmt.Sprintf(`(?m)^%s`, regexp.QuoteMeta(p))).ReplaceAllLiteral(b, nil)
	f, err := build.ParseDefault("", b)
	if err != nil {
		return b
	}
	build.Walk(f, u.visit)
	b = build.Format(f)
	// Remove trailing newline since endPattern above hasn’t matched it.
	if i := len(b) - 1; i >= 0 && b[i] == '\n' {
		b = b[:i]
	}
	return regexp.MustCompile(`(?m)^`).ReplaceAllLiteral(b, []byte(p))
}

func (u *Updater) visit(x build.Expr, stack []build.Expr) {
	c := x.Comment()
	for _, l := range [][]build.Comment{c.Before, c.Suffix, c.After} {
		for i, c := range l {
			l[i].Token = datePattern.ReplaceAllLiteralString(c.Token, u.date)
		}
	}
	a, ok := x.(*build.AssignExpr)
	if !ok {
		return
	}
	i, ok := a.LHS.(*build.Ident)
	if !ok {
		return
	}
	switch i.Name {
	case "commit":
		r, ok := a.RHS.(*build.StringExpr)
		if !ok || !hashPattern.MatchString(r.Value) {
			break
		}
		r.Value = u.refHash.String()
	case "urls":
		r, ok := a.RHS.(*build.ListExpr)
		if !ok || len(r.List) != 1 {
			break
		}
		s, ok := r.List[0].(*build.StringExpr)
		if !ok {
			break
		}
		m := urlPattern.FindStringSubmatch(s.Value)
		if m == nil {
			break
		}
		s.Value = m[1] + u.refHash.String() + m[2]
	case "sha256":
		r, ok := a.RHS.(*build.StringExpr)
		if !ok || !hashPattern.MatchString(r.Value) {
			break
		}
		r.Value = u.archiveHash.String()
	case "strip_prefix":
		r, ok := a.RHS.(*build.StringExpr)
		if !ok {
			break
		}
		m := stripPrefixPattern.FindStringSubmatch(r.Value)
		if m == nil {
			break
		}
		r.Value = m[1] + u.refHash.String()
	}
}

var (
	datePattern        = regexp.MustCompile(`20\d\d-\d\d-\d\d`)
	urlPattern         = regexp.MustCompile(`^(.+?/)[[:xdigit:]]*(\.zip)$`)
	hashPattern        = regexp.MustCompile(`^[[:xdigit:]]*$`)
	stripPrefixPattern = regexp.MustCompile(`^(.*?)[[:xdigit:]]*$`)
)

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

func downloadArchive(client *http.Client, url string) (archiveHash, time.Time, error) {
	resp, err := client.Get(url)
	if err != nil {
		return archiveHash{}, time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return archiveHash{}, time.Time{}, fmt.Errorf("downloading %s resulting in HTTP status %s", url, resp.Status)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return archiveHash{}, time.Time{}, fmt.Errorf("couldn’t download %s: %w", url, err)
	}
	if err := resp.Body.Close(); err != nil {
		return archiveHash{}, time.Time{}, fmt.Errorf("couldn’t download %s: %w", url, err)
	}

	var modified time.Time
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return archiveHash{}, time.Time{}, fmt.Errorf("couldn’t download %s: %w", url, err)
	}
	for _, f := range r.File {
		if baseDirPattern.MatchString(f.Name) {
			modified = f.Modified
			break
		}
	}

	return sha256.Sum256(b), modified, nil
}

var baseDirPattern = regexp.MustCompile(`^[^/]+/$`)

type archiveHash [sha256.Size]byte

func (h archiveHash) String() string {
	return hex.EncodeToString(h[:])
}
