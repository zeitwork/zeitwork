package main

import "zeitfun/pkg/pgtonats"

func main() {
	err := pgtonats.NewPGToNats()
	if err != nil {
		panic(err)
	}
}
