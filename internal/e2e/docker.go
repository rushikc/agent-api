package e2e

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type dockerEng struct {
	hc   *http.Client
	base string
}

func NewDocker() (Engine, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
		home, _ := os.UserHomeDir()
		for _, p := range []string{
			filepath.Join(home, ".orbstack", "run", "docker.sock"),
			filepath.Join(home, ".colima", "default", "docker.sock"),
			filepath.Join(home, ".docker", "run", "docker.sock"),
		} {
			if _, err := os.Stat(p); err == nil {
				host = "unix://" + p
				break
			}
		}
	}
	var hc *http.Client
	base := "http://docker"
	if strings.HasPrefix(host, "unix://") {
		sock := strings.TrimPrefix(host, "unix://")
		hc = &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", sock)
				},
			},
		}
	} else {
		u, err := url.Parse(host)
		if err != nil {
			return nil, err
		}
		base = u.Scheme + "://" + u.Host
		hc = &http.Client{Timeout: 0}
	}
	eng := &dockerEng{hc: hc, base: base}
	if err := eng.Ping(); err != nil {
		return nil, err
	}
	return eng, nil
}

func Ping() error {
	eng, err := NewDocker()
	if err != nil {
		return ErrEngineDown
	}
	return eng.Ping()
}

func (d *dockerEng) Ping() error {
	if d == nil || d.hc == nil {
		return ErrEngineDown
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+"/_ping", nil)
	if err != nil {
		return ErrEngineDown
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return ErrEngineDown
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ErrEngineDown
	}
	return nil
}

func (d *dockerEng) RuntimeName() string { return DetectRuntime() }

func (d *dockerEng) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return d.hc.Do(req)
}

func (d *dockerEng) HasImage(ctx context.Context, tag string) bool {
	resp, err := d.do(ctx, http.MethodGet, "/images/"+url.PathEscape(tag)+"/json", nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func (d *dockerEng) BuildImage(ctx context.Context, dockerfileDir, dockerfile, tag string, w io.Writer) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = filepath.Walk(dockerfileDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dockerfileDir, path)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		_ = tw.WriteHeader(&tar.Header{Name: rel, Size: int64(len(b)), Mode: int64(info.Mode().Perm())})
		_, _ = tw.Write(b)
		return nil
	})
	_ = tw.Close()
	q := url.Values{"t": {tag}, "dockerfile": {dockerfile}, "rm": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+"/build?"+q.Encode(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := d.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if w == nil {
		w = io.Discard
	}
	err = DecodeBuildStream(resp.Body, w)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("build: %s", resp.Status)
	}
	return err
}

func (d *dockerEng) CreateNetwork(ctx context.Context, name string) (string, error) {
	resp, err := d.do(ctx, http.MethodPost, "/networks/create", map[string]any{"Name": name})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct{ Id string }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("network create: %s", resp.Status)
	}
	return out.Id, nil
}

func (d *dockerEng) RemoveNetwork(ctx context.Context, id string) error {
	resp, err := d.do(ctx, http.MethodDelete, "/networks/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (d *dockerEng) EnsureVolume(ctx context.Context, name string) error {
	resp, err := d.do(ctx, http.MethodPost, "/volumes/create", map[string]any{"Name": name})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (d *dockerEng) CreateContainer(ctx context.Context, spec ContainerSpec) (string, error) {
	var env []string
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}
	host := map[string]any{"NetworkMode": spec.Network, "Binds": spec.Binds}
	if spec.Memory > 0 {
		host["Memory"] = spec.Memory
	}
	if spec.CPUs > 0 {
		host["NanoCpus"] = int64(spec.CPUs * 1e9)
	}
	body := map[string]any{
		"Image":      spec.Image,
		"Cmd":        spec.Cmd,
		"Env":        env,
		"Hostname":   spec.Hostname,
		"WorkingDir": spec.WorkDir,
		"HostConfig": host,
	}
	q := ""
	if spec.Name != "" {
		q = "?name=" + url.QueryEscape(spec.Name)
	}
	resp, err := d.do(ctx, http.MethodPost, "/containers/create"+q, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("create container: %s %s", resp.Status, b)
	}
	var out struct{ Id string }
	_ = json.Unmarshal(b, &out)
	return out.Id, nil
}

func (d *dockerEng) StartContainer(ctx context.Context, id string) error {
	resp, err := d.do(ctx, http.MethodPost, "/containers/"+id+"/start", map[string]any{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("start: %s", resp.Status)
	}
	return nil
}

func (d *dockerEng) WaitContainer(ctx context.Context, id string) (int, error) {
	resp, err := d.do(ctx, http.MethodPost, "/containers/"+id+"/wait", nil)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	var out struct {
		StatusCode int `json:"StatusCode"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.StatusCode, nil
}

func (d *dockerEng) Logs(ctx context.Context, id string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+"/containers/"+id+"/logs?stdout=1&stderr=1&follow=1", nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("logs: %s", resp.Status)
	}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_ = DemuxDockerLogs(resp.Body, pw)
		_ = resp.Body.Close()
	}()
	return pr, nil
}

func (d *dockerEng) CopyTo(ctx context.Context, id, dest string, tar io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, d.base+"/containers/"+id+"/archive?path="+url.QueryEscape(dest), tar)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := d.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("copy to: %s", resp.Status)
	}
	return nil
}

func (d *dockerEng) CopyFrom(ctx context.Context, id, src string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+"/containers/"+id+"/archive?path="+url.QueryEscape(src), nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("copy from: %s", resp.Status)
	}
	return resp.Body, nil
}

func (d *dockerEng) RemoveContainer(ctx context.Context, id string) error {
	resp, err := d.do(ctx, http.MethodDelete, "/containers/"+id+"?force=1&v=1", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (d *dockerEng) ContainerIP(ctx context.Context, id string) (string, error) {
	resp, err := d.do(ctx, http.MethodGet, "/containers/"+id+"/json", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var ins struct {
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string
			}
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&ins); err != nil {
		return "", err
	}
	for _, n := range ins.NetworkSettings.Networks {
		if n.IPAddress != "" {
			return n.IPAddress, nil
		}
	}
	return "", fmt.Errorf("no ip")
}

func (d *dockerEng) PruneImages(ctx context.Context, prefix string) error {
	resp, err := d.do(ctx, http.MethodGet, "/images/json?all=1", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var imgs []struct{ RepoTags []string }
	_ = json.NewDecoder(resp.Body).Decode(&imgs)
	for _, im := range imgs {
		for _, t := range im.RepoTags {
			if strings.HasPrefix(t, prefix) {
				r, err := d.do(ctx, http.MethodDelete, "/images/"+url.PathEscape(t)+"?force=1", nil)
				if err == nil {
					_ = r.Body.Close()
				}
			}
		}
	}
	return nil
}

func (d *dockerEng) PruneVolumes(ctx context.Context, prefix string) error {
	resp, err := d.do(ctx, http.MethodGet, "/volumes", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Volumes []struct{ Name string }
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	for _, v := range out.Volumes {
		if strings.HasPrefix(v.Name, prefix) {
			r, err := d.do(ctx, http.MethodDelete, "/volumes/"+v.Name+"?force=1", nil)
			if err == nil {
				_ = r.Body.Close()
			}
		}
	}
	return nil
}
