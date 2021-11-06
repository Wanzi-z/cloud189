package pkg

type Cmd interface {
	Sign()
	Login()
	Ls(path string)
	Cp(paths ...string)
	Mv(paths ...string)
	Rm(paths ...string)
	Mkdir(clouds ...string) error
	Up(cloud string, locals ...string)
	Dl(local string, clouds ...string)
}
