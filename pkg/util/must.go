package util

func Must(err error, msg string) {
	if err != nil {
		panic(msg + ": " + err.Error())
	}
}
