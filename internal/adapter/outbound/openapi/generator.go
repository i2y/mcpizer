package openapi

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"mcp-bridge/internal/domain"
	"mcp-bridge/internal/usecase"

	"github.com/getkin/kin-openapi/openapi3"
	// "mcp-bridge/internal/usecase" // Needed if we generate InvocationDetails here
)

// ToolGenerator implements the usecase.ToolGenerator interface for OpenAPI schemas.
type ToolGenerator struct {
	// config options? e.g., naming conventions, default namespace, default host
	DefaultHost string // Example: Allow setting a default host for invocation
	logger      *slog.Logger
}

// NewToolGenerator creates a new OpenAPI ToolGenerator.
func NewToolGenerator(defaultHost string, logger *slog.Logger) *ToolGenerator {
	return &ToolGenerator{
		DefaultHost: defaultHost,
		logger:      logger.With("component", "openapi_generator"),
	}
}

// Generate converts an OpenAPI document into MCP Tools and corresponding InvocationDetails.
func (g *ToolGenerator) Generate(schema domain.APISchema) ([]domain.Tool, []usecase.InvocationDetails, error) {
	log := g.logger.With(slog.String("source", schema.Source))
	log.Info("Generating tools from OpenAPI schema")

	doc, ok := schema.ParsedData.(*openapi3.T)
	if !ok || doc == nil {
		log.Error("Invalid or missing parsed OpenAPI document in APISchema")
		return nil, nil, fmt.Errorf("invalid or missing parsed OpenAPI document in APISchema")
	}

	var tools []domain.Tool
	var detailsList []usecase.InvocationDetails
	// TODO: Determine a good namespace strategy. Use API title? Hardcode? Configurable?
	namespace := sanitizeName(doc.Info.Title)
	if namespace == "" {
		namespace = "openapi"
	}
	log = log.With(slog.String("namespace", namespace))

	// Determine host: Use config, or try from doc.Servers
	host := g.DefaultHost
	if host == "" && len(doc.Servers) > 0 {
		// Use the first server URL as host. More robust logic might be needed.
		firstServerURL, err := url.Parse(doc.Servers[0].URL)
		if err == nil {
			host = firstServerURL.Scheme + "://" + firstServerURL.Host
			// TODO: Handle potential variables in server URL?
		} else {
			log.Warn("Warning: could not parse host from first server URL", slog.String("server_url", doc.Servers[0].URL), slog.Any("error", err))
		}
	}
	if host == "" {
		log.Warn("Warning: No host could be determined for OpenAPI schema. Invocation details might be incomplete.")
		// Consider returning an error if host is essential?
	}
	log.Info("Determined host for generation", slog.String("host", host))

	// Iterate through paths and operations to create tools
	generatedCount := 0
	skippedCount := 0
	for path, pathItem := range doc.Paths.Map() {
		if pathItem == nil {
			continue
		}
		for method, operation := range pathItem.Operations() {
			if operation == nil {
				continue
			}

			toolName := generateToolName(namespace, path, method, operation)
			log := log.With(slog.String("path", path), slog.String("method", method), slog.String("tool_name", toolName))

			description := operation.Description
			if description == "" {
				description = operation.Summary // Use summary if description is empty
			}
			if description == "" {
				description = fmt.Sprintf("Executes %s %s", method, path) // Fallback description
			}

			inputSchema, err := g.generateInputSchema(log, operation.Parameters, operation.RequestBody)
			if err != nil {
				log.Warn("Warning: skipping tool due to input schema generation error", slog.Any("error", err))
				skippedCount++
				continue
			}

			outputSchema, err := g.generateOutputSchema(log, operation.Responses)
			if err != nil {
				log.Warn("Warning: skipping tool due to output schema generation error", slog.Any("error", err))
				skippedCount++
				continue
			}

			tool := domain.Tool{
				Name:         toolName,
				Description:  description,
				InputSchema:  *inputSchema,
				OutputSchema: outputSchema, // Might be nil
			}
			tools = append(tools, tool)

			// Generate InvocationDetails
			details, err := g.generateInvocationDetails(log, host, path, method, operation)
			if err != nil {
				log.Warn("Warning: skipping tool due to invocation details generation error", slog.Any("error", err))
				// Remove the tool we just added if details generation failed?
				tools = tools[:len(tools)-1]
				skippedCount++
				continue
			}
			detailsList = append(detailsList, *details)
			generatedCount++
			log.Debug("Successfully generated tool and details")
		}
	}

	log.Info("Finished generating tools from OpenAPI schema",
		slog.Int("generated_count", generatedCount),
		slog.Int("skipped_count", skippedCount))
	return tools, detailsList, nil
}

// generateToolName creates a unique and descriptive name for the tool.
// Example strategy: {namespace}-{operationId} or {namespace}-{method}-{path parts}
func generateToolName(namespace, path, method string, op *openapi3.Operation) string {
	if op.OperationID != "" {
		return fmt.Sprintf("%s-%s", namespace, sanitizeName(op.OperationID))
	}

	// Fallback: use method and path
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	var nameParts []string
	nameParts = append(nameParts, namespace, strings.ToLower(method))
	for _, part := range pathParts {
		if !strings.HasPrefix(part, "{") && !strings.HasSuffix(part, "}") {
			nameParts = append(nameParts, sanitizeName(part))
		}
	}
	return strings.Join(nameParts, "-")
}

// generateInputSchema combines parameters and request body into a single JSON Schema.
func (g *ToolGenerator) generateInputSchema(log *slog.Logger, params openapi3.Parameters, requestBody *openapi3.RequestBodyRef) (*domain.JSONSchemaProps, error) {
	props := make(map[string]domain.JSONSchemaProps)
	var required []string

	// Process parameters (path, query, header, cookie)
	for _, paramRef := range params {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		param := paramRef.Value
		if param.Schema == nil || param.Schema.Value == nil {
			log.Warn("Warning: parameter has no schema", slog.String("param_name", param.Name), slog.String("param_in", param.In))
			continue
		}
		// Only include query and path params in the primary input schema typically.
		// Headers/cookies might be handled differently (e.g., via config or separate invocation metadata).
		if param.In == openapi3.ParameterInQuery || param.In == openapi3.ParameterInPath {
			paramSchema, err := g.convertSchemaRef(log, param.Schema)
			if err != nil {
				return nil, fmt.Errorf("error converting schema for parameter %s: %w", param.Name, err)
			}
			// TODO: Add parameter description to schema description?
			props[param.Name] = *paramSchema
			if param.Required {
				required = append(required, param.Name)
			}
		}
	}

	// Process request body
	if requestBody != nil && requestBody.Value != nil && requestBody.Value.Content != nil {
		// Prefer application/json
		jsonContent := requestBody.Value.Content.Get("application/json")
		if jsonContent != nil && jsonContent.Schema != nil && jsonContent.Schema.Value != nil {
			bodySchemaRef := jsonContent.Schema
			bodySchema, err := g.convertSchemaRef(log, bodySchemaRef)
			if err != nil {
				return nil, fmt.Errorf("error converting request body schema: %w", err)
			}

			if bodySchema.Type == "object" && bodySchema.Properties != nil {
				// Merge properties from body schema into the main properties map
				// This assumes a flat structure for parameters + body fields.
				// A more structured approach might nest the body under a specific key.
				for name, prop := range bodySchema.Properties {
					if _, exists := props[name]; exists {
						// Handle potential name collision (e.g., param 'id' and body field 'id')
						// Option: prefix body fields, error out, or let one overwrite.
						log.Warn("Warning: Name collision for input field", slog.String("field_name", name))
					} else {
						props[name] = prop
					}
				}
				// Merge required fields from body schema
				required = append(required, bodySchema.Required...)
			} else {
				// If the body is not an object (e.g., plain string, array), need a strategy.
				// Option: Wrap it in a key, e.g., {"body": ...}. For now, add it as a special key.
				if _, exists := props["requestBody"]; exists {
					return nil, fmt.Errorf("cannot represent non-object request body when 'requestBody' key is already used by a parameter")
				}
				props["requestBody"] = *bodySchema
				if requestBody.Value.Required {
					required = append(required, "requestBody")
				}
			}
		} else {
			// Handle other content types or lack of schema if necessary
			log.Warn("Warning: Request body found but application/json schema is missing or invalid.")
		}
	}

	// Remove duplicates from required list
	required = uniqueStrings(required)

	finalSchema := &domain.JSONSchemaProps{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
	return finalSchema, nil
}

// generateOutputSchema finds the most suitable response (e.g., 200 OK with JSON)
// and converts its schema.
func (g *ToolGenerator) generateOutputSchema(log *slog.Logger, responses *openapi3.Responses) (*domain.JSONSchemaProps, error) {
	if responses == nil || responses.Map() == nil {
		return nil, nil // No output schema defined
	}

	// Prioritize 200 or 201 response, then other 2xx
	var successResponse *openapi3.ResponseRef
	statusCodes := []string{"200", "201"}
	for _, code := range statusCodes {
		if respRef, ok := responses.Map()[code]; ok {
			successResponse = respRef
			break
		}
	}
	if successResponse == nil {
		// Look for any 2xx response
		for code, respRef := range responses.Map() {
			if strings.HasPrefix(code, "2") {
				successResponse = respRef
				break
			}
		}
	}

	if successResponse == nil || successResponse.Value == nil || successResponse.Value.Content == nil {
		log.Debug("Warning: No suitable success response found or it has no content")
		return nil, nil // No suitable success response found or it has no content
	}

	// Prefer application/json content
	jsonContent := successResponse.Value.Content.Get("application/json")
	if jsonContent == nil || jsonContent.Schema == nil || jsonContent.Schema.Value == nil {
		// Consider text/plain or other types? For now, only JSON.
		log.Debug("Warning: No JSON schema found for success response")
		return nil, nil // No JSON schema found for success response
	}

	outputSchema, err := g.convertSchemaRef(log, jsonContent.Schema)
	if err != nil {
		return nil, fmt.Errorf("error converting success response schema: %w", err)
	}

	return outputSchema, nil
}

// convertSchemaRef converts an openapi3.SchemaRef into a domain.JSONSchemaProps.
// This is recursive and handles basic types, objects, arrays, and enums.
func (g *ToolGenerator) convertSchemaRef(log *slog.Logger, ref *openapi3.SchemaRef) (*domain.JSONSchemaProps, error) {
	if ref == nil || ref.Value == nil {
		// Represent empty schema as an empty object? Or a special type?
		// Returning an empty object schema for now.
		log.Debug("Converting nil schema reference to empty object schema")
		return &domain.JSONSchemaProps{Type: "object", Properties: map[string]domain.JSONSchemaProps{}}, nil
		// Alternative: return nil, fmt.Errorf("schema reference or value is nil")
	}
	schema := ref.Value

	// Handle Type field (*openapi3.Types which is *[]string)
	var schemaType string
	if schema.Type != nil && len(*schema.Type) > 0 {
		// Take the first type if multiple are specified
		schemaType = (*schema.Type)[0]
		if len(*schema.Type) > 1 {
			log.Warn("Warning: Multiple schema types found", slog.Any("types", *schema.Type), slog.String("using_type", schemaType))
		}
	}

	props := domain.JSONSchemaProps{
		Type:   schemaType,
		Format: schema.Format,
		Enum:   schema.Enum,
		// TODO: Map other fields like description, default, validation constraints
	}

	switch schemaType { // Switch on the string representation
	case "object":
		props.Properties = make(map[string]domain.JSONSchemaProps)
		props.Required = schema.Required
		for name, propRef := range schema.Properties {
			if propRef == nil {
				continue
			}
			propSchema, err := g.convertSchemaRef(log, propRef)
			if err != nil {
				return nil, fmt.Errorf("error converting property '%s': %w", name, err)
			}
			props.Properties[name] = *propSchema
		}
	case "array":
		if schema.Items != nil {
			itemSchema, err := g.convertSchemaRef(log, schema.Items)
			if err != nil {
				return nil, fmt.Errorf("error converting array items: %w", err)
			}
			props.Items = itemSchema
		} else {
			// Array without items definition - represent as array of any type?
			// props.Items = &domain.JSONSchemaProps{} // Or maybe type object?
			log.Warn("Warning: Array schema found without 'items' definition.")
		}
	case "string", "number", "integer", "boolean":
		// Basic types, already handled by setting props.Type
	case "":
		// Type not specified or was nil.
		// JSON Schema doesn't have a direct 'any' type. Common practice is omitting 'type'.
		// Or sometimes using an empty object schema {}.
		// Let's omit type for now.
		props.Type = "" // Explicitly empty, might need handling by consumer
	default:
		// Unsupported type?
		log.Warn("Warning: Unsupported schema type encountered", slog.String("unsupported_type", schemaType))
		// Treat as string? or empty object?
		props.Type = "string" // Fallback to string
	}

	return &props, nil
}

// generateInvocationDetails creates the details needed to invoke the API endpoint.
func (g *ToolGenerator) generateInvocationDetails(log *slog.Logger, host, path, method string, op *openapi3.Operation) (*usecase.InvocationDetails, error) {
	details := usecase.InvocationDetails{
		Type:         "http", // Or potentially "connect_http" if we can detect Connect patterns
		Host:         host,
		HTTPMethod:   strings.ToUpper(method),
		HTTPPath:     path,
		PathParams:   []string{},
		QueryParams:  []string{},
		HeaderParams: make(map[string]string), // TODO: Add static headers from spec (e.g., security schemes)?
		ContentType:  "application/json",      // Default assumption
	}

	// Extract parameter names by location
	for _, paramRef := range op.Parameters {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		param := paramRef.Value
		switch param.In {
		case openapi3.ParameterInPath:
			details.PathParams = append(details.PathParams, param.Name)
		case openapi3.ParameterInQuery:
			details.QueryParams = append(details.QueryParams, param.Name)
		case openapi3.ParameterInHeader:
			// How to handle header params? Are they static or dynamic?
			// If static values are defined, add to HeaderParams map.
			// If dynamic, maybe add to a separate list like QueryParams?
			// For now, just note the name exists.
			log.Debug("Warning: Header parameter found, invocation support may vary.", slog.String("param_name", param.Name))
		case openapi3.ParameterInCookie:
			// Cookie params are generally not handled via tool inputs.
			log.Debug("Warning: Cookie parameter found, skipping for invocation details.", slog.String("param_name", param.Name))
		}
	}

	// Determine BodyParam and ContentType
	if op.RequestBody != nil && op.RequestBody.Value != nil && op.RequestBody.Value.Content != nil {
		// Prefer application/json
		jsonContent := op.RequestBody.Value.Content.Get("application/json")
		if jsonContent != nil && jsonContent.Schema != nil && jsonContent.Schema.Value != nil {
			bodySchema := jsonContent.Schema.Value
			details.ContentType = "application/json"

			// Check the first type if specified
			var bodySchemaType string
			if bodySchema.Type != nil && len(*bodySchema.Type) > 0 {
				bodySchemaType = (*bodySchema.Type)[0]
			}

			if bodySchemaType == "object" {
				// If body is an object, assume parameters not in path/query/header map to body fields.
				// A single `BodyParam` name might be too simple if the body expects multiple top-level fields
				// directly from the input parameters.
				// Let's leave BodyParam empty and let the invoker figure it out based on remaining params?
				details.BodyParam = "" // Indicate complex body construction needed
			} else {
				// If body is a primitive/array, assume it maps to a single input param named "requestBody".
				details.BodyParam = "requestBody"
			}
		} else {
			// Handle other content types (e.g., form-urlencoded, plain text) if needed
			// For now, stick to JSON or no body.
			details.ContentType = ""
		}
	} else {
		// No request body defined
		details.ContentType = ""
	}

	return &details, nil
}

// --- Helpers ---

// sanitizeName removes characters unsuitable for identifiers and replaces them.
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	// Replace non-alphanumeric characters with hyphen
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", ".", "-")
	name = replacer.Replace(name)
	// Remove consecutive hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	// Remove leading/trailing hyphens
	name = strings.Trim(name, "-")
	return name
}

// uniqueStrings removes duplicate strings from a slice.
func uniqueStrings(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	j := 0
	for _, v := range input {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		input[j] = v
		j++
	}
	return input[:j]
}

// TODO: Add function generateInvocationDetails
/*
func generateInvocationDetails(path, method string, op *openapi3.Operation) usecase.InvocationDetails {
	details := usecase.InvocationDetails{
		Type:       "http",
		HTTPMethod: method,
		HTTPPath:   path,
		// Extract path, query, header param names for the invoker
	}
	// Populate PathParams, QueryParams, HeaderParams, BodyParam based on op.Parameters and op.RequestBody
	return details
}
*/
