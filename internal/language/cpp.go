package language

func init() {
	Register(Spec{
		Name:     "cpp",
		Image:    "gcc:latest",
		FileName: "main.cpp",
		CompileCmd: []string{
			"g++",
			"/workspace/main.cpp",
			"-O2",
			"-o",
			"/workspace/a.out",
		},
		RunCommand: []string{
			"/workspace/a.out",
		},
	})
}
