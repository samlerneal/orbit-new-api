import { generateImage } from './api'
import { ImageStudioWorkbench } from './components/image-studio-workbench'

type ImageStudioProps = {
  embedded?: boolean
}

export function ImageStudio(props: ImageStudioProps) {
  return (
    <ImageStudioWorkbench
      embedded={props.embedded}
      requestImage={generateImage}
    />
  )
}
