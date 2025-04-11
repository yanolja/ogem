import argparse
import sys


def replace_content(content):
  auto_generated_comment = "// This file is auto-generated. Do not edit manually.\n\n"
  content = auto_generated_comment + content

  content = content.replace('''package studio''', '''package vertex''')
  content = content.replace('''
	"google.golang.org/api/option"''', '''''')
  content = content.replace(
      '''

// A unique identifier for the Gemini Studio provider
const REGION = "studio"''', '''''')
  content = content.replace('''type Endpoint struct {
	client *genai.Client
}''', '''type Endpoint struct {
	client *genai.Client
	region string
}''')
  content = content.replace(
      '''func NewEndpoint(apiKey string) (*Endpoint, error) {''',
      '''func NewEndpoint(projectId string, region string) (*Endpoint, error) {''')
  content = content.replace('''func NewEndpoint(apiKey string)''', '''func NewEndpoint(projectId string, region string)''')
  content = content.replace('''&genai.ClientConfig{
		APIKey: apiKey,
		Backend: genai.BackendGeminiAPI,
	}''', '''&genai.ClientConfig{
		Project: projectId,
		Location: region,
		Backend: genai.BackendVertexAI,
	}''')
  content = content.replace('''return &Endpoint{client: client}, nil''', '''return &Endpoint{client: client, region: region}, nil''')
  content = content.replace('''return "studio"''', '''return "vertex"''')
  content = content.replace('''func (ep *Endpoint) Region() string {
	return REGION
}''', '''func (ep *Endpoint) Region() string {
	return ep.region
}''')

  return content


def process_files(src_files, out_files):
  if len(src_files) != len(out_files):
    raise ValueError("Number of source files must match number of output files")

  for src_file, out_file in zip(src_files, out_files):
    try:
      with open(src_file, 'r') as f:
        content = f.read()

      modified_content = replace_content(content)

      with open(out_file, 'w') as f:
        f.write(modified_content)

      print(f'''Generated {out_file} successfully.''')

    except IOError as e:
      print(f'''Error processing file {src_file}: {e}''')
      sys.exit(1)


def main():
  parser = argparse.ArgumentParser(description="Generate Vertex AI files from Studio AI files")
  parser.add_argument('--srcs', nargs='+', required=True, help="Source files")
  parser.add_argument('--outs', nargs='+', required=True, help="Output files")

  args = parser.parse_args()

  process_files(args.srcs, args.outs)


if __name__ == "__main__":
  main()
