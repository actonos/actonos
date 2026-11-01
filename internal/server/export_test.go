package server

import "github.com/actonos/actonos/internal/tools"

// toolRegistryForTest exposes the registry the server was built with.
func (s *Server) toolRegistryForTest() *tools.ToolRegistry { return s.toolReg }
