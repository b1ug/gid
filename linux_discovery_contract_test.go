package gid

import (
	"bytes"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Compile the actual Go discovery entry and final filter against a fake C
// boundary. This runs on non-Linux hosts too, without importing the native
// backend, enumerating the host, or changing a production function variable.
func TestLinuxDiscoveryEntryWithSoftwareNativeBoundary(t *testing.T) {
	var source bytes.Buffer
	source.WriteString(linuxDiscoverySoftwareBoundary)
	for path, names := range map[string][]string{
		"gid_linux.go": {"Devices", "devicesByID"},
		"gid.go":       {"FindDevices", "filterDeviceIDs", "drainDevices"},
	} {
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			var declaration *ast.FuncDecl
			for _, item := range file.Decls {
				if function, ok := item.(*ast.FuncDecl); ok && function.Recv == nil && function.Name.Name == name {
					declaration = function
				}
			}
			if declaration == nil {
				t.Fatalf("missing production function %s in %s", name, path)
			}
			if err := format.Node(&source, fileSet, declaration); err != nil {
				t.Fatal(err)
			}
			source.WriteByte('\n')
		}
	}
	source.WriteString(linuxDiscoverySoftwareAssertions)
	temporary, err := ioutil.TempDir("", "gid-linux-discovery-software-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(temporary)
	path := filepath.Join(temporary, "main.go")
	if err := ioutil.WriteFile(path, source.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "run", path)
	command.Dir = temporary
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "GO111MODULE=") || strings.HasPrefix(value, "GOWORK=") || strings.HasPrefix(value, "CGO_ENABLED=") || strings.HasPrefix(value, "GOFLAGS=") {
			continue
		}
		command.Env = append(command.Env, value)
	}
	command.Env = append(command.Env, "GO111MODULE=off", "GOWORK=off", "CGO_ENABLED=0", "GOFLAGS=")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("software boundary: %v\n%s", err, output)
	} else if string(output) != "software discovery cases=8 PASS; host HID calls=0\n" {
		t.Fatalf("unexpected software result: %s", output)
	}
}

func TestLinuxDiscoveryBuildSelection(t *testing.T) {
	for _, test := range []struct {
		os, arch         string
		cgo              bool
		native, fallback bool
	}{
		{"linux", "amd64", true, true, false},
		{"linux", "arm64", true, true, false},
		{"android", "arm64", true, true, false},
		{"linux", "amd64", false, false, true},
		{"android", "arm64", false, false, true},
		{"darwin", "arm64", true, false, false},
		{"darwin", "arm64", false, false, true},
		{"ios", "arm64", true, false, true},
		{"windows", "amd64", true, false, true},
		{"windows", "amd64", false, false, true},
		{"freebsd", "amd64", true, false, true},
	} {
		context := build.Default
		context.GOOS, context.GOARCH, context.CgoEnabled = test.os, test.arch, test.cgo
		pkg, err := context.ImportDir(".", 0)
		if err != nil {
			t.Fatal(err)
		}
		selected := make(map[string]bool)
		for _, name := range append(pkg.GoFiles, pkg.CgoFiles...) {
			selected[name] = true
		}
		for _, file := range []struct {
			name string
			want bool
		}{{"gid_linux.go", test.native}, {"matching_fallback.go", test.fallback}} {
			if got := selected[file.name]; got != file.want {
				t.Errorf("%s/%s cgo=%t %s selected=%t want=%t", test.os, test.arch, test.cgo, file.name, got, file.want)
			}
		}
	}
}

const linuxDiscoverySoftwareBoundary = `package main
import ("fmt"; "reflect"; "sync")
type DeviceInfo struct { Path string; VendorID, ProductID, VersionNumber uint16; SerialNumber, Product, Manufacturer string }
type nativeInfo struct { next *nativeInfo; path string; vendor_id, product_id, release_number uint16; serial_number, product_string, manufacturer_string *string }
const maxDeviceChannelSize = 2
var enumerateLock sync.Mutex
var requests [][2]uint16
var frees int
var empty bool
var fixture = []DeviceInfo{
 {Path:"keyboard",VendorID:1,ProductID:2},
 {Path:"blink-first",VendorID:0x27b8,ProductID:0x01ed,SerialNumber:"B",Product:"blink",Manufacturer:"ThingM",VersionNumber:0x204},
 {Path:"vendor-zero",ProductID:0x01ed},
 {Path:"product-zero",VendorID:0x27b8},
 {Path:"both-zero"},
 {Path:"max",VendorID:0xffff,ProductID:0xffff},
 {Path:"blink-second",VendorID:0x27b8,ProductID:0x01ed,SerialNumber:"A"},
}
var C = struct {
 ushort func(uint16) uint16
 hid_enumerate func(uint16,uint16) *nativeInfo
 hid_free_enumeration func(*nativeInfo)
 GoString func(string) string
}{func(n uint16)uint16{return n}, func(v,p uint16)*nativeInfo{
 requests=append(requests,[2]uint16{v,p})
 if empty {return nil}
 var head *nativeInfo
 for i:=len(fixture)-1;i>=0;i--{
  info:=fixture[i]
  if v!=0&&v!=info.VendorID || p!=0&&p!=info.ProductID {continue}
  node:=&nativeInfo{next:head,path:info.Path,vendor_id:info.VendorID,product_id:info.ProductID,release_number:info.VersionNumber}
  if info.SerialNumber!=""{value:=info.SerialNumber;node.serial_number=&value}
  if info.Product!=""{value:=info.Product;node.product_string=&value}
  if info.Manufacturer!=""{value:=info.Manufacturer;node.manufacturer_string=&value}
  head=node
 }
 return head
},func(*nativeInfo){frees++},func(s string)string{return s}}
func wcharTToString(s *string)(string,error){return *s,nil}
`

const linuxDiscoverySoftwareAssertions = `
func collect(ch <-chan *DeviceInfo)[]DeviceInfo{
 var got []DeviceInfo
 for info:=range ch{got=append(got,*info)}
 return got
}
func main(){
 for _,ids:=range [][2]uint16{{0x27b8,0x01ed},{0,0x01ed},{0x27b8,0},{0,0},{0xffff,0xffff},{0x1234,0x5678}}{
  before:=frees
  got:=collect(FindDevices(ids[0],ids[1]))
  var want []DeviceInfo
  candidates:=false
  for _,info:=range fixture{
   if (ids[0]==0||ids[0]==info.VendorID)&&(ids[1]==0||ids[1]==info.ProductID){candidates=true}
   if ids[0]==info.VendorID&&ids[1]==info.ProductID{want=append(want,info)}
  }
  if !reflect.DeepEqual(got,want){panic(fmt.Sprintf("exact result %v got=%+v want=%+v",ids,got,want))}
  if requests[len(requests)-1]!=ids{panic("IDs did not reach native boundary unchanged")}
  wantFrees:=before;if candidates{wantFrees++}
  if frees!=wantFrees{panic("native list was not freed exactly once")}
 }
 if got:=collect(Devices());!reflect.DeepEqual(got,fixture)||requests[len(requests)-1]!=[2]uint16{0,0}{panic("generic Devices narrowed")}
 before:=frees;empty=true
 if got:=collect(FindDevices(0x27b8,0x01ed));len(got)!=0||frees!=before{panic("empty native result cleanup")}
 fmt.Println("software discovery cases=8 PASS; host HID calls=0")
}
`
