package main

import (
	"log"
	"os"
	"strings"

	"github.com/jhump/protoreflect/desc/protoparse"
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

func main() {
	log.Println("started...")
	projectPath := "C:\\Users\\g.kucherenko\\git\\server\\Microservices" //os.Args[1]
	log.Printf("scanning '%s'\n", projectPath)

	protoChan := make(chan string)
	go func() {
		defer close(protoChan)
		scanDirectory(projectPath, ".proto", protoChan)
	}()
	methods := parseProto(protoChan)

	codeChan := make(chan string)
	linkChan := make(chan linkInfo)
	go func() {
		defer close(codeChan)
		scanDirectory(projectPath, ".cs", codeChan)
	}()
	go func() {
		defer close(linkChan)
		parseCode(methods, codeChan, linkChan)
	}()

	//links := make([]linkInfo, 0, 0)
	serviceLinks := make(map[linkService]map[string]struct{}, 0)
	services := make(map[string]struct{}, 0)
	for v := range linkChan {
		log.Printf("OUTPUT: %v\n", v)
		key := linkService{v.SourceServiceName, v.TargetServiceName}
		_, exists := serviceLinks[key]
		if !exists {
			serviceLinks[key] = make(map[string]struct{}, 0)

		}
		serviceLinks[key][v.MethodName] = struct{}{}
		services[v.SourceServiceName] = struct{}{}
		services[v.TargetServiceName] = struct{}{}
	}
	log.Println(services)
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
		//if !strings.Contains(v, "C:\\Users\\g.kucherenko\\git\\server\\Microservices\\ServerSearchService\\ServerSearchService.Daemon\\Providers\\GameServiceProvider.cs") {
		//	continue
		//}
		b, err := os.ReadFile(v)
		if err != nil {
			log.Printf("Failed to open code file: %v\n", err)
		}
		content := string(b)
		//log.Println(content)
		content = strings.ReplaceAll(content, "\r\n", " ")
		content = strings.ReplaceAll(content, "\t", " ")
		words := strings.Split(content, " ")

		namespace := ""
		clients := make(map[string]struct{}, 0)
		for i, w := range words {
			w := strings.Trim(w, ";")
			if w == "namespace" {
				namespace = strings.Trim(words[i+1], ";")
			}
			partWords := strings.Split(w, ".")
			for _, pw := range partWords {
				for _, m := range methods {
					if m.ServiceName == pw {
						clients[strings.Trim(words[i+1], ";")] = struct{}{}
					}
				}
			}
		}

		for _, w := range words {
			for c := range clients {
				for _, m := range methods {
					if strings.HasPrefix(w, c+m.MethodName+"(") || w == c+m.MethodName || strings.HasPrefix(w, c+m.MethodName+"Async(") || w == c+m.MethodName+"Async" {
						output <- linkInfo{strings.Split(namespace, ".")[0], strings.Split(m.NamespaceName, ".")[1], m.MethodName}
					}
				}

			}
		}

		//fmt.Printf("%s: %s\n", v, namespace)
		//log.Printf("CLIENTS: %v\n", clients)
	}
}
