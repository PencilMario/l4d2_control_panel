package crashreports

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"unicode/utf8"
)

const multipartMemoryLimit = 8 << 20

// SubmitHandler accepts both the Accelerator pre-submit form and the final
// multipart report upload on the same endpoint.
func (m *Manager) SubmitHandler(w http.ResponseWriter, r *http.Request) {
	if !m.authorizeRequest(w, r) {
		return
	}
	contentType := r.Header.Get("Content-Type")
	mediaType, _, parseErr := mime.ParseMediaType(contentType)
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/") {
		if parseErr != nil || mediaType != "multipart/form-data" {
			writeProtocolError(w, http.StatusBadRequest, "invalid_multipart_request")
			return
		}
		m.handleUpload(w, r)
		return
	}
	m.handlePreSubmit(w, r)
}

func (m *Manager) SymbolHandler(w http.ResponseWriter, r *http.Request) {
	if !m.authorizeRequest(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	defer cleanupMultipartForm(r)
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		writeProtocolError(w, protocolParseErrorStatus(err), "invalid_symbol_request")
		return
	}
	instanceID, err := m.resolveInstanceRequest(r.Context(), formValue(r.MultipartForm, "ServerID"), formValue(r.MultipartForm, "GameDirectory"))
	if err != nil {
		writeProtocolError(w, instanceAuthorizationStatus(err), instanceAuthorizationCode(err))
		return
	}

	symbol, closeSymbol, err := requiredSymbol(r.MultipartForm)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "symbol_file_required")
		return
	}
	defer closeSymbol()
	input := SymbolInput{
		UserID:           formValue(r.MultipartForm, "UserID"),
		ExtensionVersion: formValue(r.MultipartForm, "ExtensionVersion"),
		ServerID:         formValue(r.MultipartForm, "ServerID"),
		PresubmitToken:   formValue(r.MultipartForm, "PresubmitToken"),
		InstanceID:       instanceID,
		Platform:         formValue(r.MultipartForm, "platform"),
		Architecture:     formValue(r.MultipartForm, "architecture"),
		DebugIdentifier:  formValue(r.MultipartForm, "debug_identifier"),
		CodeIdentifier:   formValue(r.MultipartForm, "code_identifier"),
		Symbol:           symbol,
	}
	if err := m.SaveSymbol(r.Context(), input); err != nil {
		writeProtocolError(w, protocolErrorStatus(err), protocolErrorCode(err))
		return
	}
	writeProtocolResponse(w, http.StatusOK, "OK")
}

func (m *Manager) BinaryHandler(w http.ResponseWriter, r *http.Request) {
	if !m.authorizeRequest(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	defer cleanupMultipartForm(r)
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		writeProtocolError(w, protocolParseErrorStatus(err), "invalid_binary_request")
		return
	}
	instanceID, err := m.resolveInstanceRequest(r.Context(), formValue(r.MultipartForm, "ServerID"), formValue(r.MultipartForm, "GameDirectory"))
	if err != nil {
		writeProtocolError(w, instanceAuthorizationStatus(err), instanceAuthorizationCode(err))
		return
	}
	file, err := requiredMultipartFile(r.MultipartForm, "code_file")
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "code_file_required")
		return
	}
	defer file.Close()
	filename := r.MultipartForm.File["code_file"][0].Filename
	if _, err := m.SaveBinary(r.Context(), BinaryInput{
		UserID:           formValue(r.MultipartForm, "UserID"),
		ExtensionVersion: formValue(r.MultipartForm, "ExtensionVersion"),
		ServerID:         formValue(r.MultipartForm, "ServerID"),
		PresubmitToken:   formValue(r.MultipartForm, "PresubmitToken"),
		InstanceID:       instanceID,
		Platform:         formValue(r.MultipartForm, "platform"),
		Architecture:     formValue(r.MultipartForm, "architecture"),
		DebugIdentifier:  formValue(r.MultipartForm, "debug_identifier"),
		CodeIdentifier:   formValue(r.MultipartForm, "code_identifier"),
		CodeFileName:     filename,
		CodeFile:         file,
	}); err != nil {
		writeProtocolError(w, protocolErrorStatus(err), protocolErrorCode(err))
		return
	}
	writeProtocolResponse(w, http.StatusOK, "OK")
}

func (m *Manager) authorizeRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProtocolError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return false
	}
	if !sourceIsLocal(r.RemoteAddr) {
		writeProtocolError(w, http.StatusForbidden, "local_source_required")
		return false
	}
	if !m.Configured() {
		writeProtocolError(w, http.StatusServiceUnavailable, "crash report receiver disabled")
		return false
	}
	if !m.Authorized(r.URL.Query().Get("token")) {
		writeProtocolError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func (m *Manager) handlePreSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxCrashSignatureBytes+64<<10)
	if err := r.ParseForm(); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_presubmit_request")
		return
	}
	signature := r.Form.Get("CrashSignature")
	if !utf8.ValidString(signature) || strings.IndexByte(signature, 0) >= 0 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_crash_signature")
		return
	}
	instanceID, err := m.resolveInstanceRequest(r.Context(), r.Form.Get("ServerID"), "")
	if err != nil {
		writeProtocolError(w, instanceAuthorizationStatus(err), instanceAuthorizationCode(err))
		return
	}
	response, err := m.PreSubmitContext(r.Context(), PreSubmitInput{
		UserID:           r.Form.Get("UserID"),
		ExtensionVersion: r.Form.Get("ExtensionVersion"),
		ServerID:         r.Form.Get("ServerID"),
		CrashSignature:   signature,
		InstanceID:       instanceID,
	})
	if err != nil {
		writeProtocolError(w, protocolErrorStatus(err), protocolErrorCode(err))
		return
	}
	writeProtocolResponse(w, http.StatusOK, response)
}

func (m *Manager) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	defer cleanupMultipartForm(r)
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		writeProtocolError(w, protocolParseErrorStatus(err), "invalid_multipart_request")
		return
	}
	instanceID, err := m.resolveInstanceRequest(r.Context(), formValue(r.MultipartForm, "ServerID"), formValue(r.MultipartForm, "GameDirectory"))
	if err != nil {
		writeProtocolError(w, instanceAuthorizationStatus(err), instanceAuthorizationCode(err))
		return
	}

	dump, err := requiredMultipartFile(r.MultipartForm, "upload_file_minidump")
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "minidump_required")
		return
	}
	defer dump.Close()
	metadata, err := requiredMultipartFile(r.MultipartForm, "upload_file_metadata")
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "metadata_required")
		return
	}
	defer metadata.Close()
	report, err := m.Receive(r.Context(), UploadInput{
		UserID:           formValue(r.MultipartForm, "UserID"),
		GameDirectory:    formValue(r.MultipartForm, "GameDirectory"),
		ExtensionVersion: formValue(r.MultipartForm, "ExtensionVersion"),
		ServerID:         formValue(r.MultipartForm, "ServerID"),
		PresubmitToken:   formValue(r.MultipartForm, "PresubmitToken"),
		InstanceID:       instanceID,
		Minidump:         dump,
		Metadata:         metadata,
	})
	if err != nil {
		writeProtocolError(w, protocolErrorStatus(err), protocolErrorCode(err))
		return
	}
	writeProtocolResponse(w, http.StatusOK, "OK|"+report.ID)
}

func requiredMultipartFile(form *multipart.Form, name string) (multipart.File, error) {
	files := form.File[name]
	if len(files) != 1 {
		return nil, fmt.Errorf("%s must contain exactly one file", name)
	}
	return files[0].Open()
}

func requiredSymbol(form *multipart.Form) (io.Reader, func(), error) {
	values := form.Value["symbol_file"]
	files := form.File["symbol_file"]
	if len(values) == 1 && len(files) == 0 {
		return strings.NewReader(values[0]), func() {}, nil
	}
	if len(values) != 0 || len(files) != 1 {
		return nil, nil, errors.New("symbol_file must contain exactly one value or file")
	}
	file, err := files[0].Open()
	if err != nil {
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}

func formValue(form *multipart.Form, name string) string {
	values := form.Value[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func cleanupMultipartForm(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

func protocolParseErrorStatus(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func sourceIsLocal(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.Unmap().IsLoopback()
}

func (m *Manager) authorizeInstanceRequest(ctx context.Context, serverID, gameDirectory string) error {
	_, err := m.resolveInstanceRequest(ctx, serverID, gameDirectory)
	return err
}

func (m *Manager) resolveInstanceRequest(ctx context.Context, serverID, gameDirectory string) (string, error) {
	if m.resolveInstance != nil {
		return m.resolveInstance(ctx, serverID, gameDirectory)
	}
	if m.authorizeInstance == nil {
		return "", ErrInstanceNotAllowed
	}
	return "", m.authorizeInstance(ctx, serverID, gameDirectory)
}

func instanceAuthorizationStatus(err error) int {
	if errors.Is(err, ErrInstanceNotAllowed) {
		return http.StatusForbidden
	}
	return http.StatusServiceUnavailable
}

func instanceAuthorizationCode(err error) string {
	if errors.Is(err, ErrInstanceNotAllowed) {
		return "instance_not_allowed"
	}
	return "instance_authorization_unavailable"
}

func protocolErrorStatus(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.Is(err, ErrInstanceNotAllowed) {
		return http.StatusForbidden
	}
	if errors.As(err, &maxBytesErr) || errors.Is(err, ErrMinidumpTooLarge) || errors.Is(err, ErrMetadataTooLarge) || errors.Is(err, ErrSymbolTooLarge) || errors.Is(err, ErrBinaryTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return http.StatusRequestTimeout
	}
	if errors.Is(err, ErrInvalidMinidump) || errors.Is(err, ErrInvalidSymbol) || errors.Is(err, ErrInvalidBinary) || errors.Is(err, ErrMetadataRequired) || errors.Is(err, ErrCrashSignatureTooLarge) || errors.Is(err, ErrTooManyModules) || errors.Is(err, ErrInvalidCrashSignature) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func protocolErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrInstanceNotAllowed):
		return "instance_not_allowed"
	case errors.Is(err, ErrMinidumpTooLarge):
		return "minidump_too_large"
	case errors.Is(err, ErrMetadataTooLarge):
		return "metadata_too_large"
	case errors.Is(err, ErrSymbolTooLarge):
		return "symbol_too_large"
	case errors.Is(err, ErrBinaryTooLarge):
		return "binary_too_large"
	case errors.Is(err, ErrInvalidMinidump):
		return "invalid_minidump"
	case errors.Is(err, ErrInvalidSymbol):
		return "invalid_symbol"
	case errors.Is(err, ErrInvalidBinary):
		return "invalid_binary"
	case errors.Is(err, ErrMetadataRequired):
		return "metadata_required"
	case errors.Is(err, ErrCrashSignatureTooLarge):
		return "crash_signature_too_large"
	case errors.Is(err, ErrTooManyModules):
		return "too_many_modules"
	case errors.Is(err, ErrInvalidCrashSignature):
		return "invalid_crash_signature"
	default:
		return "invalid_crash_report"
	}
}

func writeProtocolResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func writeProtocolError(w http.ResponseWriter, status int, code string) {
	writeProtocolResponse(w, status, code)
}
