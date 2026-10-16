package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Refuse to rewrite files that aren't plain JSON (comments/trailing commas);
// rewriting would drop the user's comments.
var errNotStrictJSON = errors.New("not plain JSON (it may contain comments)")

// object keeps JSON key order so editing one entry leaves the rest untouched.
type object []member

type member struct {
	Key   string
	Value json.RawMessage
}

func (o object) get(key string) (json.RawMessage, bool) {
	for _, m := range o {
		if m.Key == key {
			return m.Value, true
		}
	}
	return nil, false
}

func (o *object) set(key string, value json.RawMessage) {
	for i, m := range *o {
		if m.Key == key {
			(*o)[i].Value = value
			return
		}
	}
	*o = append(*o, member{Key: key, Value: value})
}

func (o *object) remove(key string) bool {
	for i, m := range *o {
		if m.Key == key {
			*o = append((*o)[:i], (*o)[i+1:]...)
			return true
		}
	}
	return false
}

func (o object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := marshal(m.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(m.Value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func parseObject(b []byte) (object, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return object{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return nil, errNotStrictJSON
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected a JSON object")
	}
	var o object
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, errNotStrictJSON
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errNotStrictJSON
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, errNotStrictJSON
		}
		o = append(o, member{Key: key, Value: v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, errNotStrictJSON
	}
	if dec.More() {
		return nil, errNotStrictJSON
	}
	return o, nil
}

type jsonServer struct {
	path       string
	serversKey string
	name       string
}

func (s jsonServer) read() (*serverEntry, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := parseObject(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	raw, ok := root.get(s.serversKey)
	if !ok {
		return nil, nil
	}
	servers, err := parseObject(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %s is not an object", s.path, s.serversKey)
	}
	entry, ok := servers.get(s.name)
	if !ok {
		return nil, nil
	}
	var e serverEntry
	if err := json.Unmarshal(entry, &e); err != nil {
		return nil, fmt.Errorf("%s: %s.%s is not an object", s.path, s.serversKey, s.name)
	}
	return &e, nil
}

func (s jsonServer) write(entry *serverEntry) (bool, error) {
	b, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	root, err := parseObject(b)
	if err != nil {
		return false, fmt.Errorf("%s: %w", s.path, err)
	}
	servers := object{}
	if raw, ok := root.get(s.serversKey); ok {
		if servers, err = parseObject(raw); err != nil {
			return false, fmt.Errorf("%s: %s is not an object", s.path, s.serversKey)
		}
	}

	if entry == nil {
		if !servers.remove(s.name) {
			return false, nil
		}
	} else {
		value, err := marshal(entry)
		if err != nil {
			return false, err
		}
		if current, ok := servers.get(s.name); ok && jsonEqual(current, value) {
			return false, nil
		}
		servers.set(s.name, value)
	}

	rawServers, err := marshal(servers)
	if err != nil {
		return false, err
	}
	root.set(s.serversKey, rawServers)
	out, err := marshal(root)
	if err != nil {
		return false, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, out, "", "  "); err != nil {
		return false, err
	}
	pretty.WriteByte('\n')
	return true, writeFileAtomic(s.path, pretty.Bytes())
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

func writeFileAtomic(path string, data []byte) error {
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
