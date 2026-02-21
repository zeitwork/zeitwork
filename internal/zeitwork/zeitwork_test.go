package zeitwork_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/zeitwork/zeitwork/internal/testsuite"
)

type Suite struct {
	testsuite.Suite
}

func Test_RunSuite(t *testing.T) {
	suite.Run(t, &Suite{testsuite.NewSuite()})
}
