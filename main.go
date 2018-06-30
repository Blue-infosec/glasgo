package main

import (
	"fmt"
	"flag"
	"go/ast"
	"go/build"
	"go/token"
	"go/parser"
	"strings"
	"os"
	"path/filepath"
)

// a global variable for the exit code.
var exitCode = 0;

var report = make(map[string]bool);

var (
	// shortens type names
	// These are the relevant AST node types to check
	// with corresponding cases
	assignStmt	*ast.AssignStmt
	binaryExpr	*ast.BinaryExpr
	callExpr	*ast.CallExpr
	compositeLit	*ast.CompositeLit
	exprStmt	*ast.ExprStmt
	forStmt		*ast.ForStmt
	funcDecl	*ast.FuncDecl
	funcLit		*ast.FuncLit
	genDecl		*ast.GenDecl
	interfaceType	*ast.InterfaceType
	rangeStmt	*ast.RangeStmt
	returnStmt	*ast.ReturnStmt
	structType	*ast.StructType
)

var (
	// checkers is a map to a map
	// the map maps AST types to maps of checker names to checker functions
	// this is to first get the functions needed for a certain type
	// and second to take just the functions we want to run.
	// refactor this to map to struct
	checkers	= make(map[ast.Node]map[string]func(*File, ast.Node))
	
)

// A map 
// File is a visitor type for the parse tree.
// it also contains the corresponding AST to a parsed file
type File struct {
	fset	*token.FileSet
	name	string
	file	*ast.File
	// a map of all registered checkers to run for each node
	checkers map[ast.Node][]func(*File, ast.Node);
}

// warnf is a formatted error printer that does not exit
// but it does set an exit code.
func warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "{insert tool name here}: "+format+"\n", args...);
	exitCode = 1;
}

// register registers the named checker function
// to be called with AST nodes of the given types.
func register(name, usage string, fn func(*File, ast.Node), types ...ast.Node) {
	report[name] = true;
	for _, typ := range types {
		m, ok := checkers[typ];
		if !ok {
			m = make(map[string]func(*File, ast.Node));
			checkers[typ] = m;
		}
		m[name] = fn;
	}
}

// Visit implements the visitor interface we need to walk the tree
// ast.Walk calls v.Visit(node)
func (f *File) Visit(node ast.Node) ast.Visitor {
	var key ast.Node
	switch node.(type) {
	case *ast.AssignStmt:
		key = assignStmt
	case *ast.BinaryExpr:
		key = binaryExpr
	case *ast.CallExpr:
		key = callExpr
	case *ast.CompositeLit:
		key = compositeLit
	case *ast.ExprStmt:
		key = exprStmt
	case *ast.ForStmt:
		key = forStmt
	case *ast.FuncDecl:
		key = funcDecl
	case *ast.FuncLit:
		key = funcLit
	case *ast.GenDecl:
		key = genDecl
	case *ast.InterfaceType:
		key = interfaceType
	case *ast.RangeStmt:
		key = rangeStmt
	case *ast.ReturnStmt:
		key = returnStmt
	case *ast.StructType:
		key = structType
	}
	// runs checkers below
	for _, fn := range f.checkers[key] {
		fn(f, node)
	}
	return f;
}

// checkPackageDir extracts the go files from a directory and passes them to 
// checkPackage for analysis
func checkPackageDir(directory string) {
	context := build.Default
	// gets build tags if any exist in order to preserve them through the coming import
	/*
	these are commented out until proof is made of being necessary
	if len(context.BuildTags) != 0 {
		warnf("build tags already set: %s," context.BuildTags);
	}
	context.BuildTags = append(tagList, context.BuildTags...);
	*/

	pkg, err := context.ImportDir(directory, 0); // 0 means no ImportMode is set i.e. default
	if err != nil {
		// no go source files
		if _, noGoSource := err.(*build.NoGoError); noGoSource {
			return;
		}
		// not considered fatal because we are recursively walking directories
		warnf("error processing directory %s, %s", directory, err);
		return;
	}
	var names []string
	names = append(names, pkg.GoFiles...);
	names = append(names, pkg.CgoFiles...);
	names = append(names, pkg.TestGoFiles...);
	/* there are other types include binary files that can be added */
	
	/* prefix each file with the directory name
	 * could use a refactor
	*/
	if directory != "." {
		for i, name := range names{
			names[i] = filepath.Join(directory, name);
		}
	}
	checkPackage(names);
}

// checkPackage runs analysis on all named files in a package.
// It parses and then runs the analysis.
// It returns the parsed package or nil.
func checkPackage(names []string) {
	var files []*File;
	var astFiles []*ast.File;
	fset := token.NewFileSet();
	var err error;
	for _, name := range names {
		// skipping using ioutil to read the file data
		// and just going to parse files directly.
		var parsedFile *ast.File;
		if strings.HasSuffix(name, ".go") {
			parsedFile, err = parser.ParseFile(fset, name, nil, parser.ParseComments)
			if err != nil {
				// warn but continue
				warnf("error: %s: %s", name, err);
				return;
			}
			astFiles = append(astFiles, parsedFile);
		}
		file := &File{
			fset:	fset,
			name:	name,
			file:	parsedFile,
		}
		files = append(files, file);
	}
	if len(astFiles) == 0 {
		return;
	}

	// Check.
	chk := make(map[ast.Node][]func(*File, ast.Node));
	for typ, set := range checkers {
		for name, fn := range set {
			// check to see if named function will be run and reported
			_, ok := report[name];
			if ok {
				chk[typ] = append(chk[typ], fn);
			}
		}
	}
	for _, file := range files {
		file.checkers = chk
		if file.file != nil {
			// Should this go in to a new function to make it more readable?
			// file.walkFile(file.name, file.file) as a method?
			fmt.Printf("Checking %s\n", file.name);
			ast.Walk(file, file.file);
		}
	}
}

// visit is for walking input directory roots
func visit(path string, info os.FileInfo, err error) error {
	if err != nil {
		warnf("directory walk error: %s", err);
		return err;
	}
	// make sure we are only dealing with directories here
	if !info.IsDir() {
		return nil
	}
	checkPackageDir(path);
	return nil;
}

func main() {
	var runOnDirs, runOnFiles bool;
	flag.Parse();

	for _, name := range flag.Args() {
		// check to see if cl argument is a directory
		f, err := os.Stat(name);
		if err != nil {
			warnf("error: %s", err);
			continue;
		}
		if f.IsDir() {
			runOnDirs = true;
		} else {
			runOnFiles = true;
		}
	}
	if runOnDirs && runOnFiles {
		// print an error
		fmt.Println("error: input arguments must not be both directories and files");
		exitCode = 1;
		os.Exit(exitCode);
	}
	if runOnDirs {
		// I want to do each directory in order
		// so I am going to loop through these regardless
		// root is a name of a directory, at the root, to be walked
		for _, root := range flag.Args() {
			filepath.Walk(root, visit);
		}
		os.Exit(exitCode);
	}
	// else they are just file names
	fileNames := flag.Args();	
	checkPackage(fileNames);
	return;
}

