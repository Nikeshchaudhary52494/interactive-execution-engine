package language

func init() {
	Register(Spec{
		Name:     "javascript",
		Image:    "node:20-alpine",
		FileName: "main.js",
		RunCommand: []string{
			"node",
			"/workspace/main.js",
		},
	})
}
