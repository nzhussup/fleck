package golang

import (
	"context"
	"os"
	"path/filepath"
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

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
