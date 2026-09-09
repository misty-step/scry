package recovery

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func remoteEndpoint(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("backup remote must be an explicit private HTTP(S) object prefix without credentials, query, or fragment in its URL")
	}
	if u.Scheme != "https" {
		host := strings.ToLower(u.Hostname())
		ip := net.ParseIP(host)
		private := host == "localhost" || strings.HasSuffix(host, ".int.exe.xyz") || (ip != nil && (ip.IsLoopback() || ip.IsPrivate()))
		if u.Scheme != "http" || !private {
			return nil, errors.New("backup remote requires HTTPS or an explicit loopback/private-network/exe integration HTTP endpoint")
		}
	}
	return u, nil
}

func privateClient(configured *http.Client) *http.Client {
	client := &http.Client{}
	if configured != nil {
		*client = *configured
	}
	if client.Timeout <= 0 || client.Timeout > 2*time.Minute {
		client.Timeout = 2 * time.Minute
	}
	// Even same-host redirects can turn PUT into GET or disclose an object to
	// a different path. The configured prefix must be the final endpoint.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

func (m *Manager) upload(ctx context.Context, record store.BackupRecord) error {
	endpoint := *m.remote
	endpoint.RawPath = strings.TrimRight(endpoint.EscapedPath(), "/") + "/" + url.PathEscape(record.RemoteKey)
	var err error
	endpoint.Path, err = url.PathUnescape(endpoint.RawPath)
	if err != nil {
		return errors.New("cannot construct private backup object path")
	}
	file, err := os.Open(record.Path)
	if err != nil {
		return fmt.Errorf("open completed backup for upload: %w", err)
	}
	defer file.Close()
	put, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), file)
	if err != nil {
		return errors.New("cannot construct private backup upload")
	}
	put.ContentLength = record.Bytes
	put.Header.Set("Content-Type", "application/zip")
	put.Header.Set("If-None-Match", "*")
	put.Header.Set("Cache-Control", "private, no-store")
	put.Header.Set("X-Amz-Content-Sha256", record.SHA256)
	digest, err := hex.DecodeString(record.SHA256)
	if err != nil {
		return errors.New("completed backup checksum is invalid")
	}
	put.Header.Set("X-Amz-Checksum-Sha256", base64.StdEncoding.EncodeToString(digest))
	m.authorize(put)
	response, err := m.client.Do(put)
	if err != nil {
		return remoteFailure(ctx, "upload")
	}
	response.Body.Close()
	if response.StatusCode == http.StatusRequestEntityTooLarge {
		return errors.New("backup exceeds the private sink's upload size limit; increase the sink limit before the next release (the completed local archive is preserved)")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("backup upload returned HTTP %d; local recovery archive remains complete", response.StatusCode)
	}
	get, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return errors.New("cannot construct private backup readback")
	}
	get.Header.Set("Accept-Encoding", "identity")
	get.Header.Set("Cache-Control", "no-cache, no-store")
	m.authorize(get)
	response, err = m.client.Do(get)
	if err != nil {
		return remoteFailure(ctx, "readback")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("backup readback returned HTTP %d; remote recovery is unverified", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != record.Bytes {
		return errors.New("backup readback length mismatch; remote recovery is unverified")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, contextReader{ctx, io.LimitReader(response.Body, record.Bytes+1)})
	if err != nil {
		return remoteFailure(ctx, "readback body")
	}
	if n != record.Bytes || hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
		return errors.New("backup readback checksum mismatch; remote recovery is unverified")
	}
	return nil
}

func (m *Manager) authorize(request *http.Request) {
	if m.cfg.RemoteToken != "" {
		request.Header.Set("Authorization", "Bearer "+m.cfg.RemoteToken)
	}
}

// net/http errors can include the entire URL, proxy credentials, or a remote
// response. Never persist/log them or response bodies as backup health text.
func remoteFailure(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("backup %s interrupted: %w", operation, err)
	}
	return fmt.Errorf("backup %s failed or timed out; remote recovery is unverified (check private endpoint reachability and authorization)", operation)
}
