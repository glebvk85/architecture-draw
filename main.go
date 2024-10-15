package main

import (
	"bufio"
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

func main() {
	log.Println("started...")
	projectPath := "C:\\Users\\g.kucherenko\\git\\server" //os.Args[1]
	log.Printf("scanning '%s'\n", projectPath)

	protoChan := make(chan string)
	go func() {
		defer close(protoChan)
		scanDirectory(projectPath, ".proto", protoChan)
	}()
	methods := parseProto(protoChan)

	codeChan := make(chan string)
	go func() {
		defer close(codeChan)
		scanDirectory(projectPath, ".cs", codeChan)
	}()
	parseCode(methods, codeChan)
}

func scanDirectory(path string, ext string, output chan<- string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == ".git" {
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
		log.Printf("parse proto %s\n", v)
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
					r = append(r, protoMethod{fdProto.GetPackage(), s.GetName(), m.GetName()})
				}
			}
		}
	}
	return r
}

func parseCode(methods []protoMethod, input <-chan string) {
	for v := range input {
		log.Printf("parse code %s\n", v)
		f, err := os.Open(v)
		if err != nil {
			log.Printf("Failed to open code file: %v\n", err)
			continue
		}

		s := bufio.NewScanner(f)
		s.Split(bufio.ScanLines)
		for s.Scan() {
			line := s.Text()
			//fmt.Println(fileScanner.Text())
		}
		f.Close()
	}
}
