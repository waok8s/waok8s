package metrics

type ValueType string

const (
	ValueInletTemperature = "inlet_temp"
	ValueDeltaPressure    = "delta_p"
)

var ValueTypes = []ValueType{
	ValueInletTemperature,
	ValueDeltaPressure,
}

type MetricData struct {
	InletTemp     float64
	DeltaPressure float64
}
