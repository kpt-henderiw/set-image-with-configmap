# set-image-with-configmap

set-image-with-configmap is a kpt function according [kpt functions](https://kpt.dev/book/02-concepts/03-functions).

The set-image function sets the image for all instances according [set-image](https://catalog.kpt.dev/set-image/v0.1/).
An enhancement was made to allow via an indirect configmap to define the set-image transform parameters. This allows
for people who consume the kpt blueprints to provide the transform parameters outside the blueprint.
If the configmap does not exist no tranformation is excecuted.
