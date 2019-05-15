package main

import (
	"encoding/json"
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"go/ast"
)

type NodeType string

const (
	_        NodeType = "(unknown)"
	TopLevel          = "topLevel"
	Folder            = "folder"
	File              = "file"
	Source            = "source"
	Package           = "package"
	Program           = "program"
	Object            = "object"
	Structure		  = "struct"
	Function		  = "func"
)

type node struct {
	Type NodeType
	Kind string
	Name string
	Loc  string
	Dir  string

	Id    string
	Label string
	Value int64
}

func gopathHandler(w http.ResponseWriter, r *http.Request) {
	gopath := os.Getenv("GOPATH")
	dirSrc := filepath.Join(gopath, "src")

	dir := dirSrc
	if str := r.FormValue("dir"); str != "" {
		dir = filepath.Join(dir, str)
	}

	//The selected node could either be a file or a directory.
	//We find this out from the dir and then treat it.
	nodes := []*node{}
	if file, err := os.Stat(dir); err != nil {
		logf("Stat for %q failed: %v", dir, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if !file.IsDir() && filepath.Ext(file.Name()) == ".go" {
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, dir, nil, 0)
		if err != nil {
			logf("ParseFile for %q failed: %v", dir, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if receiver := r.FormValue("name"); receiver != "" {
			//The selected node is a type or interface. Enumerate all reciever functions of type "name"
			var mf ast.FuncDecl
			var mi ast.TypeSpec
			ast.Inspect(af, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncDecl:
					mf = *x
					if mf.Recv != nil{
						for _, v := range mf.Recv.List {
							fmt.Print(mf.Name)
							switch xv := v.Type.(type) {
							case *ast.StarExpr:
								si, ok := xv.X.(*ast.Ident)
								if ok && si.Name == receiver {
									fmt.Println(si.Name)
									nodes = append(nodes, &node{
										Id:    fmt.Sprint(v.Pos()),
										Label: fmt.Sprintf("<code>%s</code>\n%s", "recv func", mf.Name),
										Type:  Object,
										Dir:   dir,
										Kind: "recvfunc",
									})
								}
							case *ast.Ident:
								fmt.Println(xv.Name)
							}
						}
					}
				case *ast.TypeSpec:
					mi = *x
					if fmt.Sprintf("%s",mi.Name) == receiver {
						switch mi.Type.(type){
							case *ast.InterfaceType:
								if x, ok := x.Type.(*ast.InterfaceType); ok {
									for _, x := range x.Methods.List {
										if len(x.Names) == 0 {
											continue
										}
										mname := x.Names[0].Name
										nodes = append(nodes, &node{
											Id:    fmt.Sprint(x.Pos()),
											Label: fmt.Sprintf("<code>%s</code>\n%s", "ifc func", mname),
											Type:  Object,
											Dir:   dir,
											Kind: "recvfunc",
										})
				
									}
								}
						}
					}
				default:
					fmt.Println(x)
				
				}

				return true
			})		
		} else {
			//The selcted node is a file. Enumerate all package-level functions, variables, and types
			path := dir //filepath.Join(dir, file.Name())
			loc, _ := filepath.Rel(dirSrc, path)			
			for _, v := range af.Scope.Objects {
				t := fmt.Sprintf("%s",v.Kind)
				if ts, ok := v.Decl.(*ast.TypeSpec); ok {
					if _, isIfc := ts.Type.(*ast.InterfaceType); isIfc{
						t = "interface"
					} else if _, isStr := ts.Type.(*ast.StructType); isStr{
						t = "struct"
					}
				}
				nodes = append(nodes, &node{
					Id:    fmt.Sprint(v.Pos()),
					Label: fmt.Sprintf("<code>%s</code>\n%s", t, v.Name),
					Type:  Object,
					Dir:   dir,
					Loc:   loc,
					Kind:  fmt.Sprintf("%s",t),
					Name: v.Name,
				})
			}

			for _, i := range af.Imports {
				fmt.Println(i.Path.Value)
				nodes = append(nodes, &node{
					Id:    fmt.Sprint(i.Path.Value),
					Label: fmt.Sprintf("<code>%s</code>\n%s", "import", i.Path.Value),
					Type:  Object,
					Dir:   dir,
					Loc:   loc,
					Kind:  fmt.Sprintf("%s","import"),
					Name: i.Path.Value,
				})
			}			

		}
	} else {
		//Read and list content of the directory
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			logf("ReadDir for %q failed: %v", dir, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			for _, f := range files {
				base := filepath.Base(f.Name())
				// omit hidden files
				if strings.HasPrefix(base, ".") {
					continue
				}

				path := filepath.Join(dir, f.Name())
				// omit non .go files
				if !f.IsDir() && filepath.Ext(path) != ".go" {
					continue
				}

				loc, _ := filepath.Rel(dirSrc, path)

				var typ NodeType
				if dir == dirSrc {
					typ = TopLevel
				} else if f.IsDir() {
					typ = Folder
					if pkg, err := build.Import(loc, "", 0); err == nil {
						if pkg.Name == "main" {
							typ = Program
						} else {
							typ = Package
						}
					}
				} else {
					typ = Source
				}

				nodes = append(nodes, &node{
					Id:    path,
					Label: f.Name(),
					Value: f.Size(),
					Loc:   loc,
					Dir:   dir,
					Type:  typ,
				})
			}
		}
	}

	b, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		logf("MarshalIndent failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/json")
	w.Write(b)
}
