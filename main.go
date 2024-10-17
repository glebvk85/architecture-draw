package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/pauloqueiroga/godraw"
)

type protoMethod struct {
	NamespaceName string
	ServiceName   string
	MethodName    string
}

type linkInfo struct {
	SourceServiceName string
	TargetServiceName string
	MethodName        string
}

type linkService struct {
	SourceServiceName string
	TargetServiceName string
}

type filterLinks func(source, target string) bool

func main() {
	log.Println("started...")
	mode := os.Args[1]
	projectPaths := os.Args[2:]

	protoChan := make(chan string)
	for _, p := range projectPaths {
		log.Printf("scanning '%s'\n", p)
		go func() {
			defer close(protoChan)
			scanDirectory(p, ".proto", protoChan)
		}()
	}
	methods := parseProto(protoChan)

	codeChan := make(chan string)
	linkChan := make(chan linkInfo)
	for _, p := range projectPaths {
		go func() {
			defer close(codeChan)
			scanDirectory(p, ".cs", codeChan)
		}()
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parseCode(methods, codeChan, linkChan)
		}()
	}
	go func() {
		wg.Wait()
		close(linkChan)
	}()

	//links := make([]linkInfo, 0, 0)
	serviceLinks := make(map[linkService]map[string]struct{}, 0)
	services := make(map[string]struct{}, 0)
	for v := range linkChan {
		//log.Printf("OUTPUT: %v\n", v)
		key := linkService{v.SourceServiceName, v.TargetServiceName}
		_, exists := serviceLinks[key]
		if !exists {
			serviceLinks[key] = make(map[string]struct{}, 0)

		}
		serviceLinks[key][v.MethodName] = struct{}{}
		services[v.SourceServiceName] = struct{}{}
		services[v.TargetServiceName] = struct{}{}
	}
	//log.Println(serviceLinks)
	if mode == "draw" {
		drawDiagram("general", "", false, services, serviceLinks, func(s, t string) bool { return true })
	} else if mode == "table" {
		printTable(services, serviceLinks)
	} else if mode == "draw-part" {
		for srv := range services {
			srv := srv
			drawDiagram(srv+"-In", srv, true, filterServices(srv, true, serviceLinks), serviceLinks, func(s, t string) bool { return t == srv })
			drawDiagram(srv+"-Out", srv, true, filterServices(srv, false, serviceLinks), serviceLinks, func(s, t string) bool { return s == srv })
		}
	} else {
		log.Println("Incorrect mode")
	}
}

func scanDirectory(path string, ext string, output chan<- string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == ".git" || strings.Contains(entry.Name(), ".FunctionalTests") || strings.Contains(entry.Name(), ".Tests") || strings.Contains(entry.Name(), "\\obj\\") {
				continue
			}
			scanDirectory(path+"\\"+entry.Name(), ext, output)
		}
		if strings.HasSuffix(entry.Name(), ext) {
			output <- path + "\\" + entry.Name()
		}
	}
}

func parseProto(input <-chan string) []protoMethod {
	r := make([]protoMethod, 0)
	for v := range input {
		parser := protoparse.Parser{}
		files, err := parser.ParseFiles(v)
		if err != nil {
			log.Printf("Failed to parse proto file: %v\n", err)
			continue
		}

		for _, f := range files {
			fdProto := f.AsFileDescriptorProto()
			log.Printf("%v", fdProto.GetPackage())
			for _, s := range fdProto.GetService() {
				for _, m := range s.Method {
					log.Printf("Method: %s\n", m.GetName())
					r = append(r, protoMethod{fdProto.GetPackage(), s.GetName() + "Client", "." + m.GetName()})
				}
			}
		}
	}
	return r
}

func parseCode(methods []protoMethod, input <-chan string, output chan<- linkInfo) {
	for v := range input {
		b, err := os.ReadFile(v)
		if err != nil {
			log.Printf("Failed to open code file: %v\n", err)
		}
		content := string(b)
		// log.Println(content)
		words := extractWords(content)

		namespace := ""
		clients := make(map[string]struct{}, 0)
		for i, w := range words {
			// log.Println("word", w)
			w := strings.Trim(w, ";")
			if w == "namespace" {
				namespace = strings.Trim(words[i+1], ";")
			}
			partWords := strings.Split(w, ".")
			for _, pw := range partWords {
				for _, m := range methods {
					if m.ServiceName == pw {
						clients[strings.Trim(strings.Trim(words[i+1], ";"), ",")] = struct{}{}
					}
				}
			}
		}

		if namespace == "" {
			continue
		}

		for _, w := range words {
			for c := range clients {
				for _, m := range methods {
					if strings.HasPrefix(w, c+m.MethodName+"(") || w == c+m.MethodName || strings.HasPrefix(w, c+m.MethodName+"Async(") || w == c+m.MethodName+"Async" {
						sn := strings.Split(m.NamespaceName, ".")[1]
						if sn == "Gateway" {
							sn += ("." + strings.Split(m.NamespaceName, ".")[2])
						}
						output <- linkInfo{strings.Split(namespace, ".")[0], sn, m.MethodName}
					}
				}

			}
		}

		// fmt.Printf("%s: %s\n", v, namespace)
		// log.Printf("CLIENTS: %v\n", clients)
	}
}

func drawDiagram(fileName string, centerService string, showMethods bool, services map[string]struct{}, links map[linkService]map[string]struct{}, needDraw filterLinks) {
	g := godraw.NewGraph("1")

	countServices := len(services)
	if centerService != "" {
		countServices--
	}
	step := 2 * math.Pi / float64(countServices)
	degree := 0.0
	r := 550.0
	x0 := 400.0
	y0 := 400.0
	for _, s := range getKeys(services) {
		c := godraw.NewShape(s, "1")
		var x, y float64
		if centerService == s {
			x = x0
			y = y0
		} else {
			x = x0 + r*math.Cos(degree)
			y = y0 + r*math.Sin(degree)
			degree += step
		}

		c.Geometry.X = int(x)
		c.Geometry.Y = int(y)
		c.Geometry.Height = "60"
		c.Geometry.Width = "150"
		c.Value = s
		g.Add(c)
	}

	save := false
	for s, v := range links {
		if !needDraw(s.SourceServiceName, s.TargetServiceName) {
			continue
		}
		save = true
		c := godraw.NewShape(s.SourceServiceName+s.TargetServiceName, "1")
		c.SourceID = s.SourceServiceName
		c.TargetID = s.TargetServiceName
		c.Edge = "1"
		c.Geometry = &godraw.Geometry{Relative: "1", As: "geometry"}
		if showMethods {
			c.Value = formatMethods(v)
		} else {
			c.Value = strconv.Itoa(len(v))
		}
		g.Add(c)
	}
	if !save {
		return
	}

	blob, err := xml.Marshal(g)
	if err != nil {
		log.Printf("Draw: %v", err)
	}

	_ = os.WriteFile(fmt.Sprintf("%s.drawio", fileName), blob, 0644)
}

func printTable(services map[string]struct{}, links map[linkService]map[string]struct{}) {
	f, err := os.Create("test.csv")
	if err != nil {
		log.Printf("Print: %v", err)
		return
	}

	for s := range services {
		f.WriteString(fmt.Sprintf("%s\t\t\n", s))
		for l, v := range links {
			if s == l.SourceServiceName {
				f.WriteString(fmt.Sprintf("%30s\t%s\t\n", " ", l.TargetServiceName))
				for m := range v {
					f.WriteString(fmt.Sprintf("%30s\t%30s\t%s\n", " ", " ", m))
				}
			}
		}
	}

	err = f.Close()
	if err != nil {
		log.Printf("Print: %v", err)
		return
	}
}

func extractWords(content string) []string {
	var r []string
	lines := strings.Split(content, "\r\n")
	var sb strings.Builder
	for _, str := range lines {
		sb.WriteString(strings.Trim(str, " "))
	}
	content = strings.ReplaceAll(sb.String(), "\t", " ")
	content = strings.ReplaceAll(content, "\r\n", "")
	content = strings.ReplaceAll(content, ";", " ")
	content = strings.ReplaceAll(content, "{", " { ")
	content = strings.ReplaceAll(content, "}", " } ")
	s := strings.Split(content, " ")
	for _, w := range s {
		if strings.Trim(w, " ") != "" {
			r = append(r, w)
		}
	}
	return r
}

func getKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

func formatMethods(m map[string]struct{}) string {
	var sb strings.Builder
	keys := getKeys(m)
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		sb.WriteString(strings.Trim(k, "."))
		sb.WriteString("\n")
	}
	return sb.String()
}

func filterServices(service string, incoming bool, links map[linkService]map[string]struct{}) map[string]struct{} {
	es := make(map[string]struct{})
	for k := range links {
		if incoming {
			if service != k.TargetServiceName {
				continue
			}
		} else {
			if service != k.SourceServiceName {
				continue
			}
		}
		es[k.SourceServiceName] = struct{}{}
		es[k.TargetServiceName] = struct{}{}
	}
	return es
}
