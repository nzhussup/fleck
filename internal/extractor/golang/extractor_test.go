package golang

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

func TestExtractEnvironmentAndPortPatterns(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module test.example/app\n\ngo 1.24.0\n")
	write(t, filepath.Join(dir, "main.go"), `package main
import("errors";"log";"net/http";"os")
const databaseKey = "DATABASE_URL"
func loadConfig()(string,error){ value,ok:=os.LookupEnv("RETURNED");if !ok{return "",errors.New("missing")};return value,nil }
func main(){
 db:=os.Getenv(databaseKey); if db=="" { log.Fatal("missing") }
 port:=os.Getenv("APP_PORT"); if port=="" { port="8080" }
 feature:=os.Getenv("FEATURE") == "true"; _=feature
 unknown:=os.Getenv("UNKNOWN"); _=unknown
 _,ok:=os.LookupEnv("TOKEN"); if !ok { panic("missing") }
 key:="DYNAMIC_"+unknown; _=os.Getenv(key)
 _,err:=loadConfig();if err!=nil{log.Fatal(err)}
 http.ListenAndServe(":"+port,nil)
}`)
	r, err := Extract(context.Background(), dir, dir, "app")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]model.Requirement{"DATABASE_URL": model.RequirementRequired, "APP_PORT": model.RequirementOptional, "FEATURE": model.RequirementOptional, "UNKNOWN": model.RequirementUnknown, "TOKEN": model.RequirementRequired, "RETURNED": model.RequirementRequired}
	for _, read := range r.EnvReads {
		if !read.Resolved {
			continue
		}
		if got, ok := want[read.Name]; !ok || got != read.Requirement {
			t.Errorf("read %s requirement=%s, want %s", read.Name, read.Requirement, got)
		}
		delete(want, read.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing reads: %v", want)
	}
	if len(r.ListenPorts) != 1 || r.ListenPorts[0].EnvName != "APP_PORT" || r.ListenPorts[0].DefaultPort != 8080 || r.ListenPorts[0].Resolved {
		t.Fatalf("unexpected listen port: %#v", r.ListenPorts)
	}
	foundDynamic := false
	for _, read := range r.EnvReads {
		if !read.Resolved {
			foundDynamic = true
		}
	}
	if !foundDynamic {
		t.Error("dynamic environment read was not preserved")
	}
}

func TestExtractTracksVariablesByObjectIdentity(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module test.example/identity\n\ngo 1.24.0\n")
	write(t, filepath.Join(dir, "main.go"), `package main
import("net/http";"os")
func serverA() {
 port := os.Getenv("APP_PORT")
 if port == "" { port = "8080" }
 http.ListenAndServe(":"+port, nil)
}
func serverB() {
 port := "3000"
 http.ListenAndServe(":"+port, nil)
}
func shadowed() {
 port := os.Getenv("SHADOW_PORT")
 if port == "" { port = "9090" }
 { port := "7070"; http.ListenAndServe(":"+port, nil) }
 http.ListenAndServe(":"+port, nil)
}
func main(){serverA();serverB();shadowed()}
`)

	result, err := Extract(context.Background(), dir, dir, "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ListenPorts) != 4 {
		t.Fatalf("listen ports=%#v", result.ListenPorts)
	}
	type port struct {
		env      string
		value    int
		fallback int
		resolved bool
	}
	got := make([]port, 0, len(result.ListenPorts))
	for _, listener := range result.ListenPorts {
		got = append(got, port{listener.EnvName, listener.Port, listener.DefaultPort, listener.Resolved})
	}
	sort.Slice(got, func(i, j int) bool {
		if got[i].env != got[j].env {
			return got[i].env < got[j].env
		}
		return got[i].value < got[j].value
	})
	want := []port{
		{value: 3000, resolved: true},
		{value: 7070, resolved: true},
		{env: "APP_PORT", fallback: 8080},
		{env: "SHADOW_PORT", fallback: 9090},
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("ports=%#v, want %#v", got, want)
		}
	}
}

func TestExtractClassifiesRequirednessConservatively(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module test.example/requiredness\n\ngo 1.24.0\n")
	write(t, filepath.Join(dir, "main.go"), `package main
import("log";"os")
func first() { value := os.Getenv("FIRST"); if value == "" { value = "fallback" }; _ = value }
func second() { value := os.Getenv("SECOND"); if value == "" { log.Fatal("missing") } }
func main() {
 required := os.Getenv("REQUIRED")
 if required == "" { log.Fatal("missing") }

 optional := os.Getenv("OPTIONAL")
 if optional == "" { optional = "fallback" }

 conditional := os.Getenv("CONDITIONAL")
 if conditional == "" { if os.Getenv("DEBUG") == "true" { log.Fatal("missing") } }

 unrelated := os.Getenv("UNRELATED")
 if unrelated == "" { log.Print("not configured") }
 if os.Getenv("OTHER") == "" { log.Fatal("other failure") }

 complex := os.Getenv("COMPLEX")
 if complex == "" { switch os.Getenv("MODE") { case "strict": log.Fatal("missing") } }
 wrapped := os.Getenv("WRAPPED")
 if os.Getenv("ENABLE_CHECK") == "true" { if wrapped == "" { log.Fatal("missing") } }
 guardedFallback := os.Getenv("GUARDED_FALLBACK")
 if guardedFallback == "" { if os.Getenv("SKIP") == "true" { return }; guardedFallback = "fallback" }
 guardedFatal := os.Getenv("GUARDED_FATAL")
 if guardedFatal == "" { if os.Getenv("SKIP_FATAL") == "true" { return }; log.Fatal("missing") }
 shadowed := os.Getenv("SHADOWED")
 { shadowed := ""; if shadowed == "" { shadowed = "fallback" }; _ = shadowed }
 _,_,_,_,_,_ = optional,conditional,unrelated,complex,required,shadowed
}
`)

	result, err := Extract(context.Background(), dir, dir, "app")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]model.Requirement{
		"REQUIRED":         model.RequirementRequired,
		"OPTIONAL":         model.RequirementOptional,
		"CONDITIONAL":      model.RequirementUnknown,
		"UNRELATED":        model.RequirementUnknown,
		"COMPLEX":          model.RequirementUnknown,
		"SHADOWED":         model.RequirementUnknown,
		"WRAPPED":          model.RequirementUnknown,
		"GUARDED_FALLBACK": model.RequirementUnknown,
		"GUARDED_FATAL":    model.RequirementUnknown,
		"FIRST":            model.RequirementOptional,
		"SECOND":           model.RequirementRequired,
	}
	for _, read := range result.EnvReads {
		expected, relevant := want[read.Name]
		if !relevant {
			continue
		}
		if read.Requirement != expected {
			t.Errorf("%s requirement=%s, want %s", read.Name, read.Requirement, expected)
		}
		delete(want, read.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing reads: %v", want)
	}
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
