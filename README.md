# :tada: :tada: Welcome to your new Project on GitHub :tada: :tada:

> **Note**
> This product is not officially supported by Dynatrace!

Congratulations, your project on GitHub was successfully created and you can start your Open Source Adventure!

As each adventure starts with good preparations, we also have something we would like you to do upe front.

- [ ] Read this ReadMe carefully, to get an overview of the files within your project.
- [ ] Write your own ReadMe which reflects your project
- [ ] Check if the [default community files](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/creating-a-default-community-health-file)(CONTRIBUTING.md, SECURITY.md, CODE_OF_CONDUCT.md, ..) within the organization `.github`[-project](https://github.com/dynatrace-oss/.github/) match your project's needs. If not, you can always provide your own, but we kindly ask you, that you also update those from time to time.
- [ ] Check if there is a `LICENSE` file within your project. If not, please create one containing the `Apache License 2.0`.
- [ ] Explicitly state that this project is not officially supported by Dynatrace in your ReadMe, eg. by using following lines on top of your ReadMe:

    > **Note**
    > This product is not officially supported by Dynatrace!

## How can I make my project public?

At first, the project will be private, as we (OSPO) want to ensure that you followed the guidelines and that everything is in place.

As soon as you are done with your initial commits, you can inform OSPO and we will take a close look at the project, and set it to public if we do think all guidelines are followed.

There is also some automation running, which will set projects, which do not follow the guidelines, to private.

## Provided Tools

### Markdownlint

To make it easier for the project to keep the Markdown files in a good shape, we added `markdownlint-cli` to the project.

1. with a `makefile` for easier execution locally, based on docker images, so it can be used in every environment as long as `docker` and `make` are available.
1. with a workflow for pull request verification based on the `makefile`.

The following files are part of this integration:

- `makefile`: as it contains the targets for execution
- `.markdownlint.yml`: as it contains the configuration for `markdownlint-cli`
- `.github/workflows/makefile.yml`: as it contains the GitHub Action configuration

## Licensing

We are using Apache License 2.0 as our default.

### Source Code Headers

Every file containing source code must include copyright and license
information. This includes any JS/CSS files that you might be serving out to
browsers. (This is to help well-intentioned people avoid accidental copying that
doesn't comply with the license.)

Apache header:

    Copyright 2022 Dynatrace LLC

    Licensed under the Apache License, Version 2.0 (the "License");
    you may not use this file except in compliance with the License.
    You may obtain a copy of the License at

        https://www.apache.org/licenses/LICENSE-2.0

    Unless required by applicable law or agreed to in writing, software
    distributed under the License is distributed on an "AS IS" BASIS,
    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    See the License for the specific language governing permissions and
    limitations under the License.

## Additional Questions/Remarks

If you do have additional questions/remarks, feel free to reach out to OSPO, either via slack or email.

If you think this template did not solve all your problems, please also let us know, either with a message or a pull request.
Together we can improve this template to make it easier for our future projects.
