package language

func init() {
	Register(Spec{
		Name:     "java",
		Image:    "eclipse-temurin:21-jdk-alpine",
		FileName: "Main.java",

		CompileCmd: []string{
			"javac",
			"/workspace/Main.java",
		},

		RunCommand: []string{
			"java",
			"-cp",
			"/workspace",
			"Main",
		},
	})
}
