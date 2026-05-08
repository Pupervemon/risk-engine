package http

// swaggerListRiskIPs godoc
// @Summary List risk IP insights
// @Description Returns paginated risk IP insight records. When q is provided, it is treated as an exact IP lookup.
// @Tags Risk Admin
// @Produce json
// @Param X-User-Id header string true "Authenticated user ID injected by the gateway"
// @Param X-User-Roles header string true "Authenticated role list injected by the gateway. Supports comma-separated values or JSON array. Must include 2 (teacher) or 3 (admin)."
// @Param limit query int false "Page size. Defaults to 20 and is capped at 100."
// @Param offset query int false "Pagination offset. Defaults to 0."
// @Param q query string false "Exact IP filter."
// @Success 200 {object} RiskIPListResponseDoc
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/admin/risk-ips [get]
func swaggerListRiskIPs() {}

// swaggerGetRiskIP godoc
// @Summary Get risk IP insight
// @Description Returns the aggregated insight profile for a single risk IP.
// @Tags Risk Admin
// @Produce json
// @Param X-User-Id header string true "Authenticated user ID injected by the gateway"
// @Param X-User-Roles header string true "Authenticated role list injected by the gateway. Supports comma-separated values or JSON array. Must include 2 (teacher) or 3 (admin)."
// @Param ip path string true "Risk IP address"
// @Success 200 {object} RiskIPDetailDoc
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/admin/risk-ips/{ip} [get]
func swaggerGetRiskIP() {}

// swaggerGetRiskIPEvents godoc
// @Summary List risk IP events
// @Description Returns paginated event history for a single risk IP.
// @Tags Risk Admin
// @Produce json
// @Param X-User-Id header string true "Authenticated user ID injected by the gateway"
// @Param X-User-Roles header string true "Authenticated role list injected by the gateway. Supports comma-separated values or JSON array. Must include 2 (teacher) or 3 (admin)."
// @Param ip path string true "Risk IP address"
// @Param limit query int false "Page size. Defaults to 50 and is capped at 200."
// @Param offset query int false "Pagination offset. Defaults to 0."
// @Success 200 {object} RiskIPEventsResponseDoc
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/admin/risk-ips/{ip}/events [get]
func swaggerGetRiskIPEvents() {}
