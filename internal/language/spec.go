package language

type Spec struct {
	Name       string
	Image      string
	FileName   string
	RunCommand []string
	CompileCmd []string // optional
}
