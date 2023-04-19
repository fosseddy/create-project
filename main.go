package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"bufio"
	"net/http"
	"io"
	"bytes"
	"encoding/json"
	"path"
)

type appConfig struct {
	ghUsername string
	ghApiKey string
	projDir string
}

func printUsage(stream *os.File) {
	fmt.Fprintf(
		stream,
		"Usage: %s [NAME|OPTION]\n" +
		"Creates new programming project\n" +
		"\n" +
		"NAME:\n" +
		"   project name in kebab-case\n" +
		"\n" +
		"OPTION:\n" +
		"   --help       shows this message\n" +
		"   --gen-config generates config file\n",
		os.Args[0],
	)
}

func iferr(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, msg, err)
		os.Exit(1)
	}
}

func getConfigPath() string {
	cdir, err := os.UserConfigDir()
	iferr("Failed to get user config dir: %v\n", err)
	return path.Join(cdir, "create-project", "config")
}

func (c *appConfig) isValid() bool {
	return c.ghUsername != "" && c.ghApiKey != "" && c.projDir != ""
}

func (c *appConfig) load() {
	configPath := getConfigPath()
	f, err := os.Open(configPath)
	iferr("Failed to open config file: %v\n", err)
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		kv := strings.Split(s.Text(), "=")
		k := strings.Trim(kv[0], " ")
		v := strings.Trim(kv[1], " ")

		switch k {
		case "gh_username":
			c.ghUsername = v
		case "gh_apikey":
			c.ghApiKey = v
		case "projects_dir":
			c.projDir = v
		default:
			fmt.Fprintf(os.Stderr, "Unknown config field: %s\n", k)
		}
	}

	if !c.isValid() {
		fmt.Fprintf(os.Stderr, "Config is missing required fields\n")
		os.Exit(1)
	}
}

func createRepo(name string, config *appConfig) {
	client := http.Client{}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.github.com/user/repos",
		strings.NewReader(`{"name":"` + name + `"}`),
	)
	iferr("Failed to create request: %v\n", err)

	req.Header.Add("User-Agent", "Go")
	req.Header.Add("Authorization", "token " + config.ghApiKey)

	res, err := client.Do(req)
	iferr("Failed to execute request: %v\n", err)
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		fmt.Fprintln(os.Stderr, "Failed to create repository")

		data, err := io.ReadAll(res.Body)
		iferr("Failed to read response body: %v\n", err)

		pretty := bytes.Buffer{}
		err = json.Indent(&pretty, data, "", "  ")
		iferr("Failed to indent json: %v\n", err)

		fmt.Fprintln(os.Stderr, pretty.String())
		os.Exit(1)
	}
}

func cloneRepo(name string, config *appConfig) {
	cmd := exec.Command(
		"/bin/git",
		"clone",
		fmt.Sprintf("git@github.com:%s/%s.git", config.ghUsername, name),
	)
	cmd.Dir = config.projDir
	err := cmd.Run()
	iferr("Failed to clone repository: %v\n", err)
}

func commitChanges(projPath string) {
	cmd := exec.Command("/bin/git", "add", ".")
	cmd.Dir = projPath
	err := cmd.Run()
	iferr("Failed to add changes: %v\n", err)

	cmd = exec.Command("/bin/git", "commit", "-m", "initial commit")
	cmd.Dir = projPath
	err = cmd.Run()
	iferr("Failed to commit changes: %v\n", err)

	cmd = exec.Command("/bin/git", "push", "origin", "master")
	cmd.Dir = projPath
	err = cmd.Run()
	iferr("Failed to push changes: %v\n", err)
}

func createFile(name string) *os.File {
	f, err := os.Create(name)
	iferr("Failed to create file: %v\n", err)

	err = f.Chmod(0644)
	iferr("Failed to change file mode: %v\n", err)

	return f
}

func buildMdTitle(s string) string {
	title := strings.Builder{}

	title.WriteString("# ")

	for i, word := range strings.Split(s, "-") {
		if i > 0 {
			title.WriteString(" ")
		}
		capitalized := strings.ToUpper(string(word[0])) + word[1:]
		title.WriteString(capitalized)
	}

	return title.String()
}

func createReadmeGitignore(projName string, projPath string) {
	gitignore := createFile(projPath + "/.gitignore")
	gitignore.Close()

	readme := createFile(projPath + "/README.md")
	title := buildMdTitle(projName)
	readme.WriteString(title)
	readme.Close()
}

func generateConfig() {
	configPath := getConfigPath()

	err := os.MkdirAll(path.Dir(configPath), 0700)
	iferr("failed to create config folder: %v\n", err)

	f := createFile(configPath)
	defer f.Close()

	f.WriteString(
		"gh_apikey    = github api key\n" +
		"gh_username  = github username\n" +
		"projects_dir = /absolute/path/to/dir\n",
	)
	fmt.Printf("Config created %v\n", configPath)
}

func confirm(projPath string) {
	fmt.Printf("Create project %v (y/n)\n", projPath)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	err := scanner.Err()
	iferr("Failed to scan user input: %v\n", err)

	input := scanner.Text()
	if input != "y" && input != "" {
		os.Exit(0)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Not enough arguments\n")
		printUsage(os.Stderr)
		os.Exit(1)
	}

	if strings.HasPrefix(os.Args[1], "--") {
		if os.Args[1] == "--help" {
			printUsage(os.Stdout)
			os.Exit(0)
		}

		if os.Args[1] == "--gen-config" {
			generateConfig()
			os.Exit(0)
		}

		fmt.Fprintf(os.Stderr, "Unknown option: %s\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}

	fmt.Println("Loading config file...")
	config := appConfig{}
	config.load()

	projName := os.Args[1]
	projPath := config.projDir + "/" + projName

	confirm(projPath)

	fmt.Println("Creating GitHub repository...")
	createRepo(projName, &config)

	fmt.Printf("Cloning repository into %s...\n", projPath)
	cloneRepo(projName, &config)

	fmt.Println("Creating README.md and .gitignore...")
	createReadmeGitignore(projName, projPath)

	fmt.Println("Committing changes to the repository...")
	commitChanges(projPath)

	fmt.Println("Success")
}
