package mcp

import (
	"fmt"

	mcpSDK "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server and its handlers
type Server struct {
	mcpServer *mcpSDK.Server
	handlers  *MCPHandlers
}

// NewServer creates a new MCP server instance
func NewServer(handlers *MCPHandlers) (*Server, error) {
	// Create MCP server with implementation details
	impl := &mcpSDK.Implementation{
		Name:    "Email & SMS Service",
		Version: "1.0.0",
	}

	mcpServer := mcpSDK.NewServer(impl, nil)

	server := &Server{
		mcpServer: mcpServer,
		handlers:  handlers,
	}

	// Register resources (read operations)
	if err := server.registerResources(); err != nil {
		return nil, fmt.Errorf("failed to register resources: %w", err)
	}

	// Register tools (write operations)
	if err := server.registerTools(); err != nil {
		return nil, fmt.Errorf("failed to register tools: %w", err)
	}

	return server, nil
}

// registerResources registers read-only MCP resources with the server
func (s *Server) registerResources() error {
	// Register emails list resource with RFC 6570 URI template syntax
	emailsListTemplate := &mcpSDK.ResourceTemplate{
		URITemplate: "emails://list{?page,per_page,account_id}",
		Name:        "Email List",
		Description: "List emails with pagination. Supports query params: page, per_page, account_id",
		MIMEType:    "application/json",
	}
	s.mcpServer.AddResourceTemplate(emailsListTemplate, s.handlers.HandleEmailsList)

	// Register email details resource with URI template for path parameter
	emailDetailsTemplate := &mcpSDK.ResourceTemplate{
		URITemplate: "emails://message/{messageId}",
		Name:        "Email Details",
		Description: "Get detailed information about a specific email by message ID",
		MIMEType:    "application/json",
	}
	s.mcpServer.AddResourceTemplate(emailDetailsTemplate, s.handlers.HandleEmailDetails)

	// Register SMS list resource with RFC 6570 URI template syntax
	smsListTemplate := &mcpSDK.ResourceTemplate{
		URITemplate: "sms://list{?page,per_page}",
		Name:        "SMS List",
		Description: "List SMS messages with pagination. Supports query params: page, per_page",
		MIMEType:    "application/json",
	}
	s.mcpServer.AddResourceTemplate(smsListTemplate, s.handlers.HandleSMSList)

	// Register SMS details resource with URI template for path parameter
	smsDetailsTemplate := &mcpSDK.ResourceTemplate{
		URITemplate: "sms://message/{messageSid}",
		Name:        "SMS Details",
		Description: "Get detailed information about a specific SMS message by message SID",
		MIMEType:    "application/json",
	}
	s.mcpServer.AddResourceTemplate(smsDetailsTemplate, s.handlers.HandleSMSDetails)

	return nil
}

// registerTools registers action MCP tools (write operations) with the server
func (s *Server) registerTools() error {
	// Register send_email tool
	sendEmailTool := &mcpSDK.Tool{
		Name:        "send_email",
		Description: "Send an email via a connected Gmail account",
	}
	mcpSDK.AddTool(s.mcpServer, sendEmailTool, s.handlers.SendEmail)

	// Register send_sms tool
	smsTool := &mcpSDK.Tool{
		Name:        "send_sms",
		Description: "Send an SMS via Twilio",
	}
	mcpSDK.AddTool(s.mcpServer, smsTool, s.handlers.SendSMS)

	return nil
}

// GetMCPServer returns the underlying MCP server instance
func (s *Server) GetMCPServer() *mcpSDK.Server {
	return s.mcpServer
}
