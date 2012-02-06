import logging
import os

class DirectoryImporter(object):
    file_importer = None
    logger = None

    @classmethod
    def get_instance(cls):
        from file_importer import BookFileImporter
        instance = cls()
        instance.file_importer = BookFileImporter.get_instance()
        instance.logger = logging.getLogger('directory_importer')
        return instance

    def do_import(self, path):
        for filepath in self.iterate_filepaths_under_path(path):
            self.try_to_import_file_if_not_already_imported(filepath)

    def try_to_import_file_if_not_already_imported(self, path):
        self.file_importer.try_to_import_file_if_not_already_imported(path)

    def iterate_filepaths_under_path(self, dirpath):
        for path, dirs, files in os.walk(dirpath):
            for file in files:
                yield os.path.join(path, file)
