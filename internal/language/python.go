package language

type Spec struct {
	Name       string
	Image      string
	FileName   string
	RunCommand []string
}

var Python = Spec{
	Name:       "python",
	Image:      "python:3.9-alpine",
	FileName:   "main.py",
	RunCommand: []string{"python", "-u", "main.py"},
}
