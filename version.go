package test161

import (
	"fmt"
)

type ProgramVersion struct {
	Major    uint
	Minor    uint
	Revision uint
}

var Version = ProgramVersion{
	Major:    1,
	Minor:    0,
	Revision: 0,
}

func (v ProgramVersion) String() string {
	return fmt.Sprintf("%v.%v.%v", v.Major, v.Minor, v.Revision)
}

// Returns 1 if this > other, 0 if this == other, and -1 if this < other
func (this ProgramVersion) CompareTo(other ProgramVersion) int {

	if this.Major > other.Major {
		return 1
	} else if this.Major < other.Major {
		return -1
	} else if this.Minor > other.Minor {
		return 1
	} else if this.Minor < other.Minor {
		return -1
	} else if this.Revision > other.Revision {
		return 1
	} else if this.Revision < other.Revision {
		return -1
	} else {
		return 0
	}

}
