package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Client struct {
	http *http.Client
}

type ContainerSummary struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Labels  map[string]string `json:"Labels"`
	Created int64             `json:"Created"`
}

type ContainerInspect struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	State        struct {
		StartedAt string `json:"StartedAt"`
		Status    string `json:"Status"`
	} `json:"State"`
}

type Stats struct {
	Read     string `json:"read"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage        uint64   `json:"total_usage"`
			PercpuUsage       []uint64 `json:"percpu_usage"`
			UsageInKernelmode uint64   `json:"usage_in_kernelmode"`
			UsageInUsermode   uint64   `json:"usage_in_usermode"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint64 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

func NewClient(socketPath string) *Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 30 * time.Second}}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/_ping", nil)
	return err
}

func (c *Client) ListContainers(ctx context.Context) ([]ContainerSummary, error) {
	b, err := c.do(ctx, http.MethodGet, "/containers/json?all=1", nil)
	if err != nil {
		return nil, err
	}
	var out []ContainerSummary
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) InspectContainer(ctx context.Context, id string) (ContainerInspect, error) {
	b, err := c.do(ctx, http.MethodGet, "/containers/"+id+"/json", nil)
	if err != nil {
		return ContainerInspect{}, err
	}
	var out ContainerInspect
	if err := json.Unmarshal(b, &out); err != nil {
		return ContainerInspect{}, err
	}
	return out, nil
}

func (c *Client) Stats(ctx context.Context, id string) (Stats, error) {
	b, err := c.do(ctx, http.MethodGet, "/containers/"+id+"/stats?stream=false", nil)
	if err != nil {
		return Stats{}, err
	}
	var out Stats
	if err := json.Unmarshal(b, &out); err != nil {
		return Stats{}, err
	}
	return out, nil
}

func (c *Client) Logs(ctx context.Context, id string, since time.Time, follow bool, tail int) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("stdout", "1")
	q.Set("stderr", "1")
	q.Set("timestamps", "1")
	if follow {
		q.Set("follow", "1")
	}
	if !since.IsZero() {
		q.Set("since", fmt.Sprintf("%d", since.Unix()))
	}
	if tail > 0 {
		q.Set("tail", fmt.Sprintf("%d", tail))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix"+path.Join("/containers", id, "logs")+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		defer res.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("docker logs status %d: %s", res.StatusCode, string(body))
	}
	return res.Body, nil
}

func (c *Client) Events(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/events", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("events status %d: %s", res.StatusCode, string(b))
	}
	return res.Body, nil
}

func (c *Client) do(ctx context.Context, method, p string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+p, reader)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("docker api %s %s failed: %s", method, p, msg)
	}
	return b, nil
}
