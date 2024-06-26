// COPYRIGHT 2024 FERMI NATIONAL ACCELERATOR LABORATORY
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
//
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fermitools/managed-tokens/internal/tracing"
	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// SQL statements to be used by API

// Query db actions
var (
	getSetupErrorsCountsStatement = `
	SELECT
		services.name,
		setup_errors.count
	FROM
		setup_errors
		INNER JOIN services ON services.id = setup_errors.service_id
	;
	`
	getSetupErrorsCountsByServiceStatement = `
	SELECT
		services.name,
		setup_errors.count
	FROM
		setup_errors
		INNER JOIN services ON services.id = setup_errors.service_id
	WHERE
		services.name = ?
	;
	`
	getPushErrorsCountsStatement = `
	SELECT
		services.name,
		nodes.name,
		push_errors.count
	FROM
		push_errors
		INNER JOIN services ON services.id = push_errors.service_id
		INNER JOIN nodes on nodes.id = push_errors.node_id
	;
	`
	getPushErrorsCountsByServiceStatement = `
	SELECT
		services.name,
		nodes.name,
		push_errors.count
	FROM
		push_errors
		INNER JOIN services ON services.id = push_errors.service_id
		INNER JOIN nodes on nodes.id = push_errors.node_id
	WHERE
		services.name = ?
	;
	`
	getAllServicesFromTableStatement = `
	SELECT name FROM services;
	`
	getAllNodesFromTableStatement = `
	SELECT name FROM nodes;
	`
)

// INSERT/UPDATE actions
var (
	insertIntoServicesTableStatement = `
	INSERT INTO services(name)
	VALUES
		(?)
	ON CONFLICT(name) DO NOTHING;
	`
	insertIntoNodesTableStatement = `
	INSERT INTO nodes(name)
	VALUES
		(?)
	ON CONFLICT(name) DO NOTHING;
	`
	insertOrUpdateSetupErrorsStatement = `
	INSERT INTO setup_errors(service_id, count)
	SELECT
		(SELECT services.id FROM services WHERE services.name = ?) AS service_id,
		? AS count
	ON CONFLICT(service_id) DO
		UPDATE SET count = ?
	;
	`
	insertOrUpdatePushErrorsStatement = `
	INSERT INTO push_errors(service_id, node_id, count)
	SELECT
		(SELECT services.id FROM services WHERE services.name = ?) AS service_id,
		(SELECT nodes.id FROM nodes WHERE nodes.name = ?) AS node_id,
		? as count
	ON CONFLICT(service_id, node_id) DO
		UPDATE SET count = ?
	;
	`
)

// GetAllServices queries the ManagedTokensDatabase for the registered services
// and returns a slice of strings with their names
func (m *ManagedTokensDatabase) GetAllServices(ctx context.Context) ([]string, error) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.GetAllServices")
	defer span.End()

	dataConverted, err := getNamedDimensionStringValues(ctx, m.db, getAllServicesFromTableStatement)
	if err != nil {
		tracing.LogErrorWithTrace(
			span,
			log.NewEntry(log.StandardLogger()),
			"Could not get service names from database",
			tracing.KeyValueForLog{Key: "dbLocation", Value: m.filename},
		)
		return nil, err
	}
	tracing.LogSuccessWithTrace(span, log.NewEntry(log.StandardLogger()), "Got service names from database")
	return dataConverted, nil
}

// GetAllNodes queries the ManagedTokensDatabase for the registered nodes
// and returns a slice of strings with their names
func (m *ManagedTokensDatabase) GetAllNodes(ctx context.Context) ([]string, error) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.GetAllNodes")
	defer span.End()

	dataConverted, err := getNamedDimensionStringValues(ctx, m.db, getAllNodesFromTableStatement)
	if err != nil {
		tracing.LogErrorWithTrace(
			span,
			log.NewEntry(log.StandardLogger()),
			"Could not get node names from database",
			tracing.KeyValueForLog{Key: "dbLocation", Value: m.filename},
		)
		return nil, err
	}
	tracing.LogSuccessWithTrace(span, log.NewEntry(log.StandardLogger()), "Got node names from database")
	return dataConverted, nil
}

// Setup Errors

// SetupErrorCount is an interface that wraps the Service and Count methods.  It is meant to be used both by this package and importing packages to
// retrieve service and count information about setupErrors.
type SetupErrorCount interface {
	Service() string
	Count() int
}

// setupErrorCount is an internal-facing type that implements both SetupErrorCount and insertData
type setupErrorCount struct {
	service string
	count   int
}

func (s *setupErrorCount) Service() string { return s.service }
func (s *setupErrorCount) Count() int      { return s.count }

// s.count is doubled here because of the ON CONFLICT...UPDATE clause
func (s *setupErrorCount) insertValues() []any { return []any{s.service, s.count, s.count} }

func (s *setupErrorCount) unpackDataRow(resultRow []any) (dataRowUnpacker, error) {
	// Make sure we have the right number of values
	if len(resultRow) != 2 {
		msg := "setup error data has wrong structure"
		log.Errorf("%s: %v", msg, resultRow)
		return nil, errDatabaseDataWrongStructure
	}
	// Type check each element
	serviceVal, serviceTypeOk := resultRow[0].(string)
	countVal, countTypeOk := resultRow[1].(int64)
	if !(serviceTypeOk && countTypeOk) {
		msg := "setup errors query result has wrong type.  Expected (string, int64)"
		log.Errorf("%s: got (%T, %T)", msg, resultRow[0], resultRow[1])
		return nil, errDatabaseDataWrongType
	}
	log.Debugf("Got SetupError row: %s, %d", serviceVal, countVal)
	return &setupErrorCount{serviceVal, int(countVal)}, nil
}

// GetSetupErrorsInfo queries the ManagedTokensDatabase for setup error counts.  It returns the data in the form of a slice of SetupErrorCounts
// that the caller can unpack using the interface methods Service() and Count()
func (m *ManagedTokensDatabase) GetSetupErrorsInfo(ctx context.Context) ([]SetupErrorCount, error) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.GetSetupErrorsInfo")
	span.SetAttributes(attribute.String("dbLocation", m.filename))
	defer span.End()

	funcLogger := log.WithField("dbLocation", m.filename)

	// dataConverted := make([]SetupErrorCount, 0)
	data, err := getValuesTransactionRunner(ctx, m.db, getSetupErrorsCountsStatement)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not get setup errors information from ManagedTokensDatabase")
		return nil, err
	}

	if len(data) == 0 {
		funcLogger.Debug("No setup error data in database")
		return nil, sql.ErrNoRows
	}

	// Unpack data
	unpackedData, err := unpackData[*setupErrorCount](data)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Error unpacking setupErrorCount data")
		return nil, err
	}
	convertedData := make([]SetupErrorCount, 0, len(unpackedData))
	for _, datum := range unpackedData {
		convertedData = append(convertedData, datum)
	}

	tracing.LogSuccessWithTrace(span, funcLogger, "Got setup errors information from ManagedTokensDatabase")
	return convertedData, nil
}

// GetSetupErrorsInfoByService queries the ManagedTokensDatabase for the setup errors for a specific service.  It returns the data as a SetupErrorCount that
// calling functions can unpack using the Service() or Count() functions.
func (m *ManagedTokensDatabase) GetSetupErrorsInfoByService(ctx context.Context, service string) (SetupErrorCount, error) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.GetSetupErrorsInfoByService")
	span.SetAttributes(attribute.String("service", service), attribute.String("dbLocation", m.filename))
	defer span.End()

	funcLogger := log.WithFields(log.Fields{"dbLocation": m.filename, "service": service})
	data, err := getValuesTransactionRunner(ctx, m.db, getSetupErrorsCountsByServiceStatement, service)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not get setup errors information from ManagedTokensDatabase")
		return nil, err
	}

	if len(data) == 0 {
		funcLogger.Debug("No setup error data in database")
		return nil, sql.ErrNoRows
	}

	if len(data) != 1 {
		msg := fmt.Sprintf("setup error data should only have 1 row: %v", data)
		tracing.LogErrorWithTrace(span, funcLogger, msg)
		return nil, errDatabaseDataWrongStructure
	}

	unpackedData, err := unpackData[*setupErrorCount](data)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Error unpacking setupErrorCount data")
		return nil, err
	}
	tracing.LogSuccessWithTrace(span, funcLogger, "Got setup errors information from ManagedTokensDatabase")
	return unpackedData[0], nil
}

// UpdateSetupErrorsTable updates the setup errors table of the ManagedTokens database.  The information to be modified
// in the database should be given as a slice of SetupErrorCount (setupErrorsByService)
func (m *ManagedTokensDatabase) UpdateSetupErrorsTable(ctx context.Context, setupErrorsByService []SetupErrorCount) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.UpdateSetupErrorsTable")
	span.SetAttributes(attribute.String("dbLocation", m.filename))
	defer span.End()

	funcLogger := log.WithField("dbLocation", m.filename)

	setupErrorDatumSlice := setupErrorCountInterfaceSliceToInsertValuesSlice(setupErrorsByService)

	if err := insertValuesTransactionRunner(ctx, m.db, insertOrUpdateSetupErrorsStatement, setupErrorDatumSlice); err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not update setup errors in ManagedTokensDatabase")
		return err
	}
	tracing.LogSuccessWithTrace(span, funcLogger, "Updated setup errors in ManagedTokensDatabase")
	return nil
}

// setupErrorCountInterfaceSliceToInsertValuesSlice converts a []SetupErrorCount to []insertValues
func setupErrorCountInterfaceSliceToInsertValuesSlice(setupErrorCountInterfaceSlice []SetupErrorCount) []insertValues {
	sl := make([]insertValues, 0, len(setupErrorCountInterfaceSlice))
	for _, datum := range setupErrorCountInterfaceSlice {
		sl = append(sl,
			&setupErrorCount{
				service: datum.Service(),
				count:   datum.Count(),
			})
	}
	return sl
}

// Push Errors

// PushErrorCount is an interface that wraps the Service, Node, and Count methods.  It is meant to be used both by this package and
// importing packages to retrieve service, node, and count information about pushErrors.
type PushErrorCount interface {
	Service() string
	Node() string
	Count() int
}

// pushErrorCount is an internal-facing type that implements both PushErrorCount and insertValues
type pushErrorCount struct {
	service string
	node    string
	count   int
}

func (p *pushErrorCount) Service() string { return p.service }
func (p *pushErrorCount) Node() string    { return p.node }
func (p *pushErrorCount) Count() int      { return p.count }

// p.count is doubled here because of the ON CONFLICT...UPDATE clause
func (p *pushErrorCount) insertValues() []any { return []any{p.service, p.node, p.count, p.count} }

func (p *pushErrorCount) unpackDataRow(resultRow []any) (dataRowUnpacker, error) {
	// Make sure we have the right number of values
	if len(resultRow) != 3 {
		msg := "push error data has wrong structure"
		log.Errorf("%s: %v", msg, resultRow)
		return nil, errDatabaseDataWrongStructure
	}
	// Type check each element
	serviceVal, serviceTypeOk := resultRow[0].(string)
	nodeVal, nodeTypeOk := resultRow[1].(string)
	countVal, countTypeOk := resultRow[2].(int64)
	if !(serviceTypeOk && nodeTypeOk && countTypeOk) {
		msg := "push errors query result has wrong type.  Expected (string, string, int64)"
		log.Errorf("%s: got (%T, %T, %T)", msg, resultRow[0], resultRow[1], resultRow[2])
		return nil, errDatabaseDataWrongType
	}
	log.Debugf("Got PushErrorCount row: %s, %s, %d", serviceVal, nodeVal, countVal)

	return &pushErrorCount{serviceVal, nodeVal, int(countVal)}, nil
}

// GetPushErrorsInfo queries the ManagedTokensDatabase for push error counts.  It returns the data in the form of a slice of PushErrorCounts
// that the caller can unpack using the interface methods Service(), Node(), and Count()
func (m *ManagedTokensDatabase) GetPushErrorsInfo(ctx context.Context) ([]PushErrorCount, error) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.GetPushErrorsInfo")
	defer span.End()

	funcLogger := log.WithField("dbLocation", m.filename)
	data, err := getValuesTransactionRunner(ctx, m.db, getPushErrorsCountsStatement)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not get push errors information from ManagedTokensDatabase")
		return nil, err
	}

	if len(data) == 0 {
		funcLogger.Debug("No push error data in database")
		return nil, sql.ErrNoRows
	}

	// Unpack data
	unpackedData, err := unpackData[*pushErrorCount](data)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Error unpacking pushErrorCount data")
		return nil, err
	}
	convertedData := make([]PushErrorCount, 0, len(unpackedData))
	for _, datum := range unpackedData {
		convertedData = append(convertedData, datum)
	}
	tracing.LogSuccessWithTrace(span, funcLogger, "Got push errors information from ManagedTokensDatabase")
	return convertedData, nil
}

// GetPushErrorsInfoByService queries the database for the push errors for a specific service.  It returns the data as a slice of PushErrorCounts
// that the caller can unpack using the Service(), Node(), and Count() interface methods.
func (m *ManagedTokensDatabase) GetPushErrorsInfoByService(ctx context.Context, service string) ([]PushErrorCount, error) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.GetPushErrorsInfoByService")
	span.SetAttributes(
		attribute.String("service", service),
		attribute.String("dbLocation", m.filename),
	)
	defer span.End()

	funcLogger := log.WithFields(log.Fields{"dbLocation": m.filename, "service": service})
	data, err := getValuesTransactionRunner(ctx, m.db, getPushErrorsCountsByServiceStatement, service)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not get push errors information from ManagedTokensDatabase")
		return nil, err
	}

	if len(data) == 0 {
		funcLogger.Debug("No push error data in database")
		return nil, sql.ErrNoRows
	}

	// Unpack data
	unpackedData, err := unpackData[*pushErrorCount](data)
	if err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not unpack data from database")
		return nil, err
	}
	convertedData := make([]PushErrorCount, 0, len(unpackedData))
	for _, datum := range unpackedData {
		convertedData = append(convertedData, datum)
	}
	tracing.LogSuccessWithTrace(span, funcLogger, "Got push errors information from ManagedTokensDatabase")
	return convertedData, nil
}

// UpdatePushErrorsTable updates the push errors table of the ManagedTokens database.  The information to be modified
// in the database should be given as a slice of PushErrorCount (pushErrorsByServiceAndNode)
func (m *ManagedTokensDatabase) UpdatePushErrorsTable(ctx context.Context, pushErrorsByServiceAndNode []PushErrorCount) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.UpdatePushErrorsTable")
	defer span.End()

	funcLogger := log.WithField("dbLocation", m.filename)
	pushErrorDatumSlice := pushErrorCountInterfaceSliceToInsertValuesSlice(pushErrorsByServiceAndNode)

	if err := insertValuesTransactionRunner(ctx, m.db, insertOrUpdatePushErrorsStatement, pushErrorDatumSlice); err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not update push errors in ManagedTokensDatabase")
		return err
	}
	tracing.LogSuccessWithTrace(span, funcLogger, "Updated push errors in ManagedTokensDatabase")
	return nil
}

// pushErrorCountInterfaceSliceToInsertValuesSlice converts a []PushErrorCount to []insertValues
func pushErrorCountInterfaceSliceToInsertValuesSlice(pushErrorCountInterfaceSlice []PushErrorCount) []insertValues {
	sl := make([]insertValues, 0, len(pushErrorCountInterfaceSlice))
	for _, datum := range pushErrorCountInterfaceSlice {
		sl = append(sl,
			&pushErrorCount{
				service: datum.Service(),
				node:    datum.Node(),
				count:   datum.Count(),
			})
	}
	return sl
}

// Dimension data

// serviceDatum is an internal type that implements the insertValues interface.  It holds the name of a service as its value.
type serviceDatum string

func (s *serviceDatum) insertValues() []any { return []any{s} }

// UpdateServices updates the services table in the ManagedTokensDatabase.  It takes a slice
// of strings for the service names, and inserts them if they don't already exist in the
// database
func (m *ManagedTokensDatabase) UpdateServices(ctx context.Context, serviceNames []string) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.UpdateServices")
	span.SetAttributes(
		attribute.String("dbLocation", m.filename),
		attribute.StringSlice("serviceNames", serviceNames),
	)
	defer span.End()

	funcLogger := log.WithField("dbLocation", m.filename)

	serviceDatumSlice := convertStringSliceToInsertValuesSlice(
		newInsertValuesFromUnderlyingString[*serviceDatum, serviceDatum],
		serviceNames,
	)

	if err := insertValuesTransactionRunner(ctx, m.db, insertIntoServicesTableStatement, serviceDatumSlice); err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not update services in ManagedTokensDatabase")
		return err
	}

	tracing.LogSuccessWithTrace(span, funcLogger, "Updated services in ManagedTokensDatabase")
	return nil
}

// nodeDatum is an internal type that implements the insertValues interface.  It holds the name of a node as its value.
type nodeDatum string

func (n *nodeDatum) insertValues() []any { return []any{n} }

// UpdateNodes updates the nodes table in the ManagedTokensDatabase.  It takes a slice
// of strings for the node names, and inserts them if they don't already exist in the
// database
func (m *ManagedTokensDatabase) UpdateNodes(ctx context.Context, nodes []string) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "db.UpdateNodes")
	span.SetAttributes(
		attribute.String("dbLocation", m.filename),
		attribute.StringSlice("nodes", nodes),
	)
	defer span.End()

	funcLogger := log.WithField("dbLocation", m.filename)

	nodesDatumSlice := convertStringSliceToInsertValuesSlice(
		newInsertValuesFromUnderlyingString[*nodeDatum, nodeDatum],
		nodes,
	)

	if err := insertValuesTransactionRunner(ctx, m.db, insertIntoNodesTableStatement, nodesDatumSlice); err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not update nodes in ManagedTokensDatabase")
		return err
	}

	tracing.LogSuccessWithTrace(span, funcLogger, "Updated nodes in ManagedTokensDatabase")
	return nil

}
