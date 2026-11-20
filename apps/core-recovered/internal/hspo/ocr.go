package hspo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const maxOCRPDF = 25 << 20

func (a *App) ocrAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		accountError(w, http.StatusMethodNotAllowed, "Phương thức không được hỗ trợ")
		return
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		accountError(w, http.StatusUnsupportedMediaType, "Cần gửi PDF dưới dạng JSON")
		return
	}
	worker, err := filepath.Abs(filepath.Join(a.Root, "HQTN_HSPO_OCR_Worker.ps1"))
	if err != nil || runtime.GOOS != "windows" {
		accountError(w, http.StatusServiceUnavailable, "OCR QA chỉ có trên Windows")
		return
	}
	if _, err := os.Stat(worker); err != nil {
		accountError(w, http.StatusServiceUnavailable, "Bản QA chưa có bộ đọc OCR cục bộ")
		return
	}
	if !a.ocrMu.TryLock() {
		accountError(w, http.StatusTooManyRequests, "Đang đọc một tệp PDF. Vui lòng thử lại khi tác vụ hiện tại hoàn tất.")
		return
	}
	defer a.ocrMu.Unlock()

	var request struct {
		PDFBase64 string `json:"pdfBase64"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(base64.StdEncoding.EncodedLen(maxOCRPDF)+4096))
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		accountError(w, http.StatusBadRequest, "Dữ liệu PDF không hợp lệ hoặc vượt 25 MB")
		return
	}
	if len(request.PDFBase64) > base64.StdEncoding.EncodedLen(maxOCRPDF) {
		accountError(w, http.StatusRequestEntityTooLarge, "PDF vượt 25 MB")
		return
	}
	pdf, err := base64.StdEncoding.DecodeString(request.PDFBase64)
	if err != nil || len(pdf) > maxOCRPDF || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		accountError(w, http.StatusBadRequest, "Tệp gửi lên phải là PDF hợp lệ, tối đa 25 MB")
		return
	}

	dir, err := os.MkdirTemp("", "hspo-qa-ocr-")
	if err != nil {
		accountError(w, http.StatusInternalServerError, "Không tạo được thư mục OCR tạm")
		return
	}
	defer os.RemoveAll(dir)
	input, output := filepath.Join(dir, "upload.pdf"), filepath.Join(dir, "result.json")
	if err := os.WriteFile(input, pdf, 0600); err != nil {
		accountError(w, http.StatusInternalServerError, "Không ghi được PDF vào thư mục OCR tạm")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Minute)
	defer cancel()
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", worker, "-PdfPath", input, "-OutPath", output)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TEMP="+dir, "TMP="+dir)
	runErr := cmd.Run()
	if ctx.Err() != nil {
		accountError(w, http.StatusGatewayTimeout, "Tác vụ OCR đã hết thời gian xử lý hoặc bị hủy")
		return
	}
	var result struct {
		OK          bool   `json:"ok"`
		Engine      string `json:"engine"`
		PageCount   int    `json:"pageCount"`
		Diagnostics struct {
			SelectedCAS            []string `json:"selectedCas"`
			TablePages             []int    `json:"tablePages"`
			TableReviewRecommended bool     `json:"tableReviewRecommended"`
		} `json:"diagnostics"`
		Pages []struct {
			Page int    `json:"page"`
			Text string `json:"text"`
		} `json:"pages"`
	}
	if runErr != nil || readJSON(output, &result) != nil || !result.OK {
		log.Printf("local QA OCR worker did not complete: %v", runErr)
		accountError(w, http.StatusUnprocessableEntity, "Bộ OCR không đọc được PDF này")
		return
	}
	jsonOut(w, http.StatusOK, result)
}
