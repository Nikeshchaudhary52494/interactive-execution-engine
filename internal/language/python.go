package language

func init() {
	Register(Spec{
		Name:     "python",
		Image:    "python:3.11-alpine",
		FileName: "main.py",
		RunCommand: []string{
			"python",
			"-u",
			"/workspace/main.py",
		},
	})
}
