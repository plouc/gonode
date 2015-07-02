package handlers

import (
	"fmt"
	"github.com/lib/pq"
	nc "github.com/rande/gonode/core"
	"github.com/spf13/afero"
	"io"
	"net/http"
)

type ExifMeta map[string]string

func UpdateMediaImageMetadata(file afero.File, written int64, meta *ImageMeta) {
	raw := make([]byte, 512)

	file.Seek(0, 0)
	file.Read(raw)
	file.Seek(0, 0)

	meta.ContentType = http.DetectContentType(raw)
	meta.Size = int(written)
}

type ImageMeta struct {
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	Size         int      `json:"size"`
	ContentType  string   `json:"content_type"`
	Length       int      `json:"length"`
	Exif         ExifMeta `json:"exif"`
	Hash         string   `json:"hash"`
	SourceStatus int      `json:"source_status"`
	SourceError  string   `json:"source_error"`
}

type Image struct {
	Reference string `json:"reference"`
	Name      string `json:"name"`
	SourceUrl string `json:"source_url"`
}

type ImageHandler struct {
	Fs afero.Fs
}

func (h *ImageHandler) GetStruct() (nc.NodeData, nc.NodeMeta) {
	return &Image{}, &ImageMeta{
		SourceStatus: nc.ProcessStatusInit,
	}
}

func (h *ImageHandler) PreInsert(node *nc.Node, m nc.NodeManager) error {
	data := node.Data.(*Image)
	meta := node.Meta.(*ImageMeta)

	if data.SourceUrl != "" && meta.SourceStatus == nc.ProcessStatusInit {
		meta.SourceStatus = nc.ProcessStatusUpdate
		meta.SourceError = ""
	}

	return nil
}

func (h *ImageHandler) PreUpdate(node *nc.Node, m nc.NodeManager) error {
	data := node.Data.(*Image)
	meta := node.Meta.(*ImageMeta)

	if data.SourceUrl != "" && meta.SourceStatus == nc.ProcessStatusInit {
		meta.SourceStatus = nc.ProcessStatusUpdate
		meta.SourceError = ""
	}

	return nil
}

func (h *ImageHandler) PostInsert(node *nc.Node, m nc.NodeManager) error {
	meta := node.Meta.(*ImageMeta)

	if meta.SourceStatus == nc.ProcessStatusUpdate {
		m.Notify("media_file_download", node.Uuid.String())
	}

	return nil
}

func (h *ImageHandler) PostUpdate(node *nc.Node, m nc.NodeManager) error {
	meta := node.Meta.(*ImageMeta)

	if meta.SourceStatus == nc.ProcessStatusUpdate {
		m.Notify("media_file_download", node.Uuid.String())
	}

	return nil
}

func (h *ImageHandler) Validate(node *nc.Node, m nc.NodeManager, errors nc.Errors) {

}

func (h *ImageHandler) GetDownloadData(node *nc.Node) *nc.DownloadData {
	meta := node.Meta.(*ImageMeta)

	data := nc.GetDownloadData()
	data.Filename = node.Name
	data.ContentType = meta.ContentType
	data.Stream = func(node *nc.Node, w io.Writer) {
		file, err := h.Fs.Open(nc.GetFileLocation(node))

		nc.PanicOnError(err)

		io.Copy(w, file)
	}

	return data
}

func (h *ImageHandler) Load(data []byte, meta []byte, node *nc.Node) error {
	return nc.HandlerLoad(h, data, meta, node)
}

func (h *ImageHandler) StoreStream(node *nc.Node, r io.Reader) (afero.File, int64, error) {
	file, written, err := nc.CopyNodeStreamToFile(h.Fs, node, r)

	UpdateMediaImageMetadata(file, written, node.Meta.(*ImageMeta))

	return file, written, err
}

type ImageDownloadListener struct {
	Fs         afero.Fs
	HttpClient nc.HttpClient
}

func (l *ImageDownloadListener) Handle(notification *pq.Notification, m nc.NodeManager) (int, error) {

	reference := nc.GetReferenceFromString(notification.Extra)

	fmt.Printf("Download binary from uuid: %s\n", notification.Extra)
	node := m.Find(reference)

	if node == nil {
		fmt.Printf("Uuid does not exist: %s\n", notification.Extra)
		return nc.PubSubListenContinue, nil
	}

	data := node.Data.(*Image)
	meta := node.Meta.(*ImageMeta)

	if meta.SourceStatus == nc.ProcessStatusDone {
		fmt.Printf("Nothing to update: %s\n", notification.Extra)

		return nc.PubSubListenContinue, nil
	}

	resp, err := l.HttpClient.Get(data.SourceUrl)

	if err != nil {
		meta.SourceStatus = nc.ProcessStatusError
		meta.SourceError = "Unable to retrieve the remote file"
		m.Save(node)

		return nc.PubSubListenContinue, err
	}

	defer resp.Body.Close()

	file, written, err := nc.CopyNodeStreamToFile(l.Fs, node, resp.Body)

	if err != nil {
		return nc.PubSubListenContinue, err
	}

	UpdateMediaImageMetadata(file, written, meta)

	meta.SourceStatus = nc.ProcessStatusDone
	m.Save(node)

	return nc.PubSubListenContinue, nil
}
