import { generateImage } from './api'
import { ImageStudioWorkbench } from './components/image-studio-workbench'

export function ImageStudio() {
  return <ImageStudioWorkbench requestImage={generateImage} />
}
