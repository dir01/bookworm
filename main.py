import argparse

parser = argparse.ArgumentParser(description='Import your books collection')
parser.add_argument(
    'directories', metavar='DIRNAME', type=str, nargs='+',
    help='Directory to import books from'
)

def setup():
    from db.base import BaseModel, engine
    BaseModel.metadata.create_all(engine)


def main():
    from directory_importer import DirectoryImporter

    setup()

    args = parser.parse_args()
    for path in args.directories:
        DirectoryImporter(path).do_import()


if __name__ == '__main__':
    main()
