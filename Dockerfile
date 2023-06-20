# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM --platform=$TARGETPLATFORM alpine:3.18.2

RUN apk add gcompat
RUN apk add --no-cache ca-certificates
# if we build on a Linux box it will use glibc ld.so but its still built
# statically and will work against musl so.. hand wave.
#RUN mkdir -p /lib64 && ln -s /lib/ld-musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2
ADD rackspace-cloud-controller-manager /bin/cloud-controller-manager

ENTRYPOINT ["/bin/cloud-controller-manager"]
