package main

type Master struct{}

func NewMaster(_ *Config) *Master {
	return &Master{}
}

func (m *Master) Run() {

}
