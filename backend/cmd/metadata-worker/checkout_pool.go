package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const defaultCheckoutCacheMaxBytes int64 = 4294967296

type checkoutIdentity struct {
	Repository       repository
	Masters          []repository
	Profile, Runtime string
}
type checkoutSet struct {
	root, key    string
	repositories []repository
	paths        []string
	bytes        int64
}
type checkoutPool struct {
	mu       sync.Mutex
	root     string
	lock     *os.File
	maxBytes int64
	serial   uint64
	idle     []*checkoutSet // oldest first
	active   map[string]bool
	changed  chan struct{}
	blocked  error
	remove   func(string) error
}

func checkoutCacheMaxBytesFromEnv() (int64, error) {
	value := os.Getenv("METADATA_WORKER_CHECKOUT_CACHE_MAX_BYTES")
	if value == "" {
		return defaultCheckoutCacheMaxBytes, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("METADATA_WORKER_CHECKOUT_CACHE_MAX_BYTES must be a nonnegative integer, got %q", value)
	}
	return n, nil
}

func openCheckoutPool(temp string, maxBytes int64) (*checkoutPool, error) {
	if maxBytes < 0 {
		return nil, errors.New("negative checkout cache budget")
	}
	if temp == "" {
		temp = os.TempDir()
	}
	p := &checkoutPool{root: filepath.Join(temp, "checkout-pool"), maxBytes: maxBytes, active: make(map[string]bool), changed: make(chan struct{}), remove: os.RemoveAll}
	if err := os.MkdirAll(p.root, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(p.root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("checkout pool must be a directory, not a symlink")
	}
	lock, err := os.OpenFile(filepath.Join(p.root, "owner.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("checkout pool already owned: %w", err)
	}
	p.lock = lock
	entries, err := os.ReadDir(p.root)
	if err == nil {
		for _, entry := range entries {
			if entry.Name() != "owner.lock" {
				if err = p.remove(filepath.Join(p.root, entry.Name())); err != nil {
					break
				}
			}
		}
	}
	if err != nil {
		p.close()
		return nil, fmt.Errorf("clean stale checkout pool: %w", err)
	}
	return p, nil
}
func (p *checkoutPool) close() error { return p.lock.Close() }
func checkoutKey(input request, commit, runtime string) string {
	main := input.Repository
	main.Commit = commit
	data, _ := json.Marshal(checkoutIdentity{Repository: main, Masters: input.Masters, Profile: input.Profile, Runtime: runtime})
	return string(data)
}
func (p *checkoutPool) signal() { close(p.changed); p.changed = make(chan struct{}) }
func (p *checkoutPool) discard(s *checkoutSet, reason string) error {
	log.Printf("metadata checkout evict reason=%s", reason)
	if err := p.remove(s.root); err != nil {
		p.blocked = fmt.Errorf("remove checkout set: %w; restart required", err)
		log.Print(p.blocked)
		return p.blocked
	}
	return nil
}
func (p *checkoutPool) acquire(ctx context.Context, input request, commit, runtime string) (*checkoutSet, error) {
	key := checkoutKey(input, commit, runtime)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.maxBytes == 0 {
		p.serial++
		key += fmt.Sprintf("/disabled/%d", p.serial)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if p.blocked != nil {
			return nil, p.blocked
		}
		if !p.active[key] {
			break
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.mu.Lock()
			return nil, ctx.Err()
		case <-changed:
		}
		p.mu.Lock()
	}
	p.active[key] = true
	for i, s := range p.idle {
		if s.key == key {
			p.idle = append(p.idle[:i], p.idle[i+1:]...)
			log.Print("metadata checkout reuse")
			return s, nil
		}
	}
	root, err := os.MkdirTemp(p.root, "set-")
	if err != nil {
		delete(p.active, key)
		p.signal()
		return nil, err
	}
	repositories := append([]repository{input.Repository}, input.Masters...)
	repositories[0].Commit = commit
	s := &checkoutSet{root: root, key: key, repositories: repositories}
	for i, repo := range repositories {
		path := filepath.Join(root, fmt.Sprintf("repo-%d", i))
		s.paths = append(s.paths, path)
		if err = checkout(repo.Path, repo.Commit, path); err != nil {
			cleanupErr := p.discard(s, "create-error")
			delete(p.active, key)
			p.signal()
			return nil, errors.Join(err, cleanupErr)
		}
	}
	log.Print("metadata checkout create")
	return s, nil
}
func restoreCheckout(path, commit string) error {
	for _, args := range [][]string{{"reset", "--hard", commit}, {"clean", "-ffdx"}} {
		cmd := exec.Command("git", append([]string{"--no-replace-objects", "--no-lazy-fetch", "-C", path}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore checkout: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	for _, check := range []struct {
		args []string
		want string
	}{{[]string{"rev-parse", "HEAD"}, commit}, {[]string{"status", "--porcelain", "--untracked-files=all", "--ignored"}, ""}} {
		output, err := exec.Command("git", append([]string{"--no-replace-objects", "--no-lazy-fetch", "-C", path}, check.args...)...).CombinedOutput()
		if err != nil || strings.TrimSpace(string(output)) != check.want {
			return fmt.Errorf("restored checkout verification failed: %s: %v", output, err)
		}
	}
	return nil
}
func allocatedBytes(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.New("allocated blocks unavailable")
		}
		size += stat.Blocks * 512
		return nil
	})
	return size, err
}
func (p *checkoutPool) release(s *checkoutSet, invalid bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer func() { delete(p.active, s.key); p.signal() }()
	if p.maxBytes == 0 {
		return p.discard(s, "disabled")
	}
	var restoreErr error
	for i, path := range s.paths {
		restoreErr = errors.Join(restoreErr, restoreCheckout(path, s.repositories[i].Commit))
	}
	if restoreErr != nil || invalid {
		return errors.Join(restoreErr, p.discard(s, "invalid"))
	}
	log.Print("metadata checkout restore")
	size, err := allocatedBytes(s.root)
	if err != nil {
		return errors.Join(err, p.discard(s, "size-error"))
	}
	s.bytes = size
	if p.maxBytes == 0 || size > p.maxBytes {
		return p.discard(s, "budget")
	}
	p.idle = append(p.idle, s)
	var total int64
	for _, entry := range p.idle {
		total += entry.bytes
	}
	for len(p.idle) > 16 || total > p.maxBytes {
		oldest := p.idle[0]
		p.idle = p.idle[1:]
		total -= oldest.bytes
		if err := p.discard(oldest, "lru"); err != nil {
			return err
		}
	}
	return nil
}
