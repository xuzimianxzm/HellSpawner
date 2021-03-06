package hsmpqexplorer

import (
	"github.com/OpenDiablo2/HellSpawner/hscommon/hsutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/OpenDiablo2/HellSpawner/hscommon/hsstate"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2mpq"
	g "github.com/ianling/giu"

	"github.com/OpenDiablo2/HellSpawner/hscommon"
	"github.com/OpenDiablo2/HellSpawner/hscommon/hsproject"
	"github.com/OpenDiablo2/HellSpawner/hsconfig"
	"github.com/OpenDiablo2/HellSpawner/hswindow/hstoolwindow"
)

type MPQExplorerFileSelectedCallback func(path *hscommon.PathEntry)

type MPQExplorer struct {
	*hstoolwindow.ToolWindow
	config               *hsconfig.Config
	project              *hsproject.Project
	fileSelectedCallback MPQExplorerFileSelectedCallback
	nodeCache            []g.Widget

	filesToOverwrite []fileToOverwrite
}

type fileToOverwrite struct {
	Path string
	Data []byte
}

func Create(fileSelectedCallback MPQExplorerFileSelectedCallback, config *hsconfig.Config, x, y float32) (*MPQExplorer, error) {
	result := &MPQExplorer{
		ToolWindow:           hstoolwindow.New("MPQ Explorer", hsstate.ToolWindowTypeMPQExplorer, x, y),
		fileSelectedCallback: fileSelectedCallback,
		config:               config,
	}

	return result, nil
}

func (m *MPQExplorer) SetProject(project *hsproject.Project) {
	m.project = project
}

func (m *MPQExplorer) Build() {
	if m.project == nil {
		return
	}

	needToShowOverwritePrompt := len(m.filesToOverwrite) > 0
	if needToShowOverwritePrompt {
		m.IsOpen(&needToShowOverwritePrompt).Layout(g.Layout{
			g.PopupModal("Overwrite File?").IsOpen(&needToShowOverwritePrompt).Layout(g.Layout{
				g.Label("File at " + m.filesToOverwrite[0].Path + " already exists. Overwrite?"),
				g.Line(
					g.Button("Overwrite").OnClick(func() {
						success := hsutil.CreateFileAtPath(m.filesToOverwrite[0].Path, m.filesToOverwrite[0].Data)
						if success {
							m.project.InvalidateFileStructure()
						}
						m.filesToOverwrite = m.filesToOverwrite[1:]
					}),
					g.Button("Cancel").OnClick(func() {
						m.filesToOverwrite = m.filesToOverwrite[1:]
					}),
				),
			})})
	} else {
		m.IsOpen(&m.Visible).
			Size(300, 400).
			Layout(g.Layout{
				g.Child("MpqExplorerContent").
					Border(false).
					Flags(g.WindowFlagsHorizontalScrollbar).
					Layout(m.getMpqTreeNodes()),
			})
	}
}

func (m *MPQExplorer) getMpqTreeNodes() []g.Widget {
	if m.nodeCache != nil {
		return m.nodeCache
	}

	wg := sync.WaitGroup{}
	result := make([]g.Widget, len(m.project.AuxiliaryMPQs))
	wg.Add(len(m.project.AuxiliaryMPQs))

	for mpqIndex := range m.project.AuxiliaryMPQs {
		go func(idx int) {
			mpq, err := d2mpq.FromFile(filepath.Join(m.config.AuxiliaryMpqPath, m.project.AuxiliaryMPQs[idx]))
			if err != nil {
				log.Fatal("failed to load mpq: ", err)
			}
			nodes := m.project.GetMPQFileNodes(mpq, m.config)
			result[idx] = m.renderNodes(nodes)

			wg.Done()
		}(mpqIndex)
	}

	wg.Wait()

	m.nodeCache = result
	return result
}

func (m *MPQExplorer) renderNodes(pathEntry *hscommon.PathEntry) g.Widget {
	if !pathEntry.IsDirectory {
		id := generatePathEntryId(pathEntry)
		return g.Layout{
			g.Selectable(pathEntry.Name + id).
				OnClick(func() {
					go m.fileSelectedCallback(pathEntry)
				}),
			g.ContextMenu("Context" + id).Layout(g.Layout{
				g.Selectable("Copy to Project").OnClick(func() {
					m.copyToProject(pathEntry)
				}),
			})}
	}

	widgets := make([]g.Widget, len(pathEntry.Children))
	hscommon.SortPaths(pathEntry)

	wg := sync.WaitGroup{}
	wg.Add(len(pathEntry.Children))

	for childIdx := range pathEntry.Children {
		go func(idx int) {
			widgets[idx] = m.renderNodes(pathEntry.Children[idx])
			wg.Done()
		}(childIdx)
	}

	wg.Wait()

	return g.TreeNode(pathEntry.Name).Layout(widgets)
}

func (m *MPQExplorer) copyToProject(pathEntry *hscommon.PathEntry) {
	data, err := pathEntry.GetFileBytes()
	if err != nil {
		log.Printf("failed to read file %s when copying to project: %s", pathEntry.FullPath, err)
		return
	}

	pathToFile := pathEntry.FullPath
	if strings.HasPrefix(pathEntry.FullPath, "data") {
		// strip "data" from the beginning of the path if it exists
		pathToFile = pathToFile[4:]
	}
	pathToFile = path.Join(m.project.GetProjectFileContentPath(), pathToFile)
	pathToFile = strings.ReplaceAll(pathToFile, "\\", "/")

	if _, err := os.Stat(pathToFile); err == nil {
		// file already exists
		fileInfo := fileToOverwrite{
			Path: pathToFile,
			Data: data,
		}
		m.filesToOverwrite = append(m.filesToOverwrite, fileInfo)
		return
	}

	success := hsutil.CreateFileAtPath(pathToFile, data)
	if success {
		m.project.InvalidateFileStructure()
	}
}

func generatePathEntryId(pathEntry *hscommon.PathEntry) string {
	return "##MPQExplorerNode_" + pathEntry.FullPath
}

func (m *MPQExplorer) Reset() {
	m.nodeCache = nil
}
