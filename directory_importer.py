import os

from db.dao import BooksDao

from file_importer import BookFileImporter


class DirectoryImporter(object):
    def __init__(self, path):
        self.path = path

    def do_import(self):
        for path in self.get_all_filenames_iter():
            self.try_to_import_file_if_not_already_imported(path)

    def try_to_import_file_if_not_already_imported(self, path):
        self.get_book_file_importer(path).try_to_import_file_if_not_already_imported()

    def get_all_filenames_iter(self):
        for path, dirs, files in os.walk(self.path):
            for file in files:
                yield os.path.join(path, file)

    def get_book_file_importer(self, path):
        book_file_importer = BookFileImporter(path)
        book_file_importer.dao = BooksDao()
        return book_file_importer
