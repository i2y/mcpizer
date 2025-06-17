package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	calculatorv1 "github.com/example/calculator/gen/calculator/v1"
)

// CalculatorService implements the calculator service
type CalculatorService struct{}

// Add performs addition
func (s *CalculatorService) Add(
	ctx context.Context,
	req *connect.Request[calculatorv1.AddRequest],
) (*connect.Response[calculatorv1.AddResponse], error) {
	result := req.Msg.A + req.Msg.B
	return connect.NewResponse(&calculatorv1.AddResponse{
		Result: result,
	}), nil
}

// Subtract performs subtraction
func (s *CalculatorService) Subtract(
	ctx context.Context,
	req *connect.Request[calculatorv1.SubtractRequest],
) (*connect.Response[calculatorv1.SubtractResponse], error) {
	result := req.Msg.A - req.Msg.B
	return connect.NewResponse(&calculatorv1.SubtractResponse{
		Result: result,
	}), nil
}

// Multiply performs multiplication
func (s *CalculatorService) Multiply(
	ctx context.Context,
	req *connect.Request[calculatorv1.MultiplyRequest],
) (*connect.Response[calculatorv1.MultiplyResponse], error) {
	result := req.Msg.A * req.Msg.B
	return connect.NewResponse(&calculatorv1.MultiplyResponse{
		Result: result,
	}), nil
}

// Divide performs division
func (s *CalculatorService) Divide(
	ctx context.Context,
	req *connect.Request[calculatorv1.DivideRequest],
) (*connect.Response[calculatorv1.DivideResponse], error) {
	if req.Msg.B == 0 {
		return connect.NewResponse(&calculatorv1.DivideResponse{
			Result: 0,
			Error:  "division by zero",
		}), nil
	}
	result := req.Msg.A / req.Msg.B
	return connect.NewResponse(&calculatorv1.DivideResponse{
		Result: result,
	}), nil
}

// Calculate evaluates a simple expression
func (s *CalculatorService) Calculate(
	ctx context.Context,
	req *connect.Request[calculatorv1.CalculateRequest],
) (*connect.Response[calculatorv1.CalculateResponse], error) {
	// Simple expression parser (only supports +, -, *, / with two operands)
	expr := strings.TrimSpace(req.Msg.Expression)
	
	// Try each operator
	operators := []string{"+", "-", "*", "/"}
	for _, op := range operators {
		parts := strings.Split(expr, op)
		if len(parts) == 2 {
			a, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			b, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			
			if err1 != nil || err2 != nil {
				return connect.NewResponse(&calculatorv1.CalculateResponse{
					Result: 0,
					Error:  fmt.Sprintf("invalid expression: %s", expr),
				}), nil
			}
			
			var result float64
			switch op {
			case "+":
				result = a + b
			case "-":
				result = a - b
			case "*":
				result = a * b
			case "/":
				if b == 0 {
					return connect.NewResponse(&calculatorv1.CalculateResponse{
						Result: 0,
						Error:  "division by zero",
					}), nil
				}
				result = a / b
			}
			
			return connect.NewResponse(&calculatorv1.CalculateResponse{
				Result: result,
			}), nil
		}
	}
	
	return connect.NewResponse(&calculatorv1.CalculateResponse{
		Result: 0,
		Error:  fmt.Sprintf("unsupported expression: %s", expr),
	}), nil
}
